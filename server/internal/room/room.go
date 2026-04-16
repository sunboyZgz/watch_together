package room

import (
	"errors"
	"sync"
)

var ErrNotHost = errors.New("only host can control playback")

type State struct {
	RoomID       string
	MediaID      string
	HostUserID   string
	Paused       bool
	PositionMs   int64
	PlaybackRate float64
	Seq          int64
}

type Room struct {
	id      string
	mu      sync.RWMutex
	clients map[*ClientConnection]struct{}
	state   State
}

// New creates a room with a minimal default playback state.
func New(id string) *Room {
	return &Room{
		id:      id,
		clients: make(map[*ClientConnection]struct{}),
		state: State{
			RoomID:       id,
			MediaID:      "",
			HostUserID:   "",
			Paused:       true,
			PositionMs:   0,
			PlaybackRate: 1.0,
			Seq:          1,
		},
	}
}

// NewCreatedRoom creates a room that already has an initial host and media binding.
func NewCreatedRoom(id string, hostUserID string, mediaID string) *Room {
	room := New(id)
	room.state.MediaID = mediaID
	room.state.HostUserID = hostUserID
	return room
}

// ID returns the stable room identifier.
func (r *Room) ID() string {
	return r.id
}

// StateSnapshot returns the current room state in a read-safe way.
func (r *Room) StateSnapshot() State {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.state
}

// Join registers a client in the room and returns the current room snapshot.
func (r *Room) Join(client *ClientConnection) State {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.clients[client] = struct{}{}
	// The first connected user becomes the initial host until later host rules are added.
	if r.state.HostUserID == "" {
		r.state.HostUserID = client.UserID()
	}
	return r.state
}

// Leave removes a client and transfers host ownership if the host disconnects.
func (r *Room) Leave(client *ClientConnection) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.clients, client)
	if r.state.HostUserID == client.UserID() {
		r.state.HostUserID = ""
		for candidate := range r.clients {
			r.state.HostUserID = candidate.UserID()
			break
		}
	}
}

// ClientCount reports how many active connections the room currently holds.
func (r *Room) ClientCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.clients)
}

// ApplyPlay updates the room's authority state for a play action and snapshots the
// active clients so the transport layer can broadcast without holding the room lock.
func (r *Room) ApplyPlay(userID string, positionMs int64) (State, []*ClientConnection, error) {
	return r.applyControl(userID, positionMs, false)
}

// ApplyPause updates the room's authority state for a pause action.
func (r *Room) ApplyPause(userID string, positionMs int64) (State, []*ClientConnection, error) {
	return r.applyControl(userID, positionMs, true)
}

// ApplySeek updates the room position while preserving the current paused flag.
func (r *Room) ApplySeek(userID string, positionMs int64) (State, []*ClientConnection, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.state.HostUserID != userID {
		return State{}, nil, ErrNotHost
	}

	r.state.PositionMs = positionMs
	r.state.Seq++
	return r.state, r.clientsSnapshotLocked(), nil
}

func (r *Room) applyControl(
	userID string,
	positionMs int64,
	paused bool,
) (State, []*ClientConnection, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.state.HostUserID != userID {
		return State{}, nil, ErrNotHost
	}

	r.state.Paused = paused
	r.state.PositionMs = positionMs
	r.state.Seq++
	return r.state, r.clientsSnapshotLocked(), nil
}

func (r *Room) clientsSnapshotLocked() []*ClientConnection {
	clients := make([]*ClientConnection, 0, len(r.clients))
	for client := range r.clients {
		clients = append(clients, client)
	}
	return clients
}
