package room

import (
	"errors"
	"math"
	"sync"
	"time"
)

var ErrNotHost = errors.New("only host can control playback")

type State struct {
	RoomID       string
	MediaID      string
	HostUserID   string
	Paused       bool
	Ended        bool
	PositionMs   int64
	PlaybackRate float64
	Seq          int64
}

type Room struct {
	id            string
	mu            sync.RWMutex
	clients       map[*ClientConnection]struct{}
	clientsByUser map[string]*ClientConnection
	state         State
	authorityAt   time.Time
	now           func() time.Time
}

type LeaveResult struct {
	State           State
	Remaining       []*ClientConnection
	HostTransferred bool
	RoomEmpty       bool
}

type JoinResult struct {
	State          State
	ReplacedClient *ClientConnection
}

// New creates a room with a minimal default playback state.
func New(id string) *Room {
	return newWithClock(id, time.Now)
}

func newWithClock(id string, now func() time.Time) *Room {
	return &Room{
		id:            id,
		clients:       make(map[*ClientConnection]struct{}),
		clientsByUser: make(map[string]*ClientConnection),
		state: State{
			RoomID:       id,
			MediaID:      "",
			HostUserID:   "",
			Paused:       true,
			Ended:        false,
			PositionMs:   0,
			PlaybackRate: 1.0,
			Seq:          1,
		},
		authorityAt: now(),
		now:         now,
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

	return r.currentStateLocked(r.now())
}

// Join registers a client in the room and replaces any previous connection from the same user.
func (r *Room) Join(client *ClientConnection) JoinResult {
	r.mu.Lock()
	defer r.mu.Unlock()

	var replacedClient *ClientConnection
	if existing := r.clientsByUser[client.UserID()]; existing != nil && existing != client {
		delete(r.clients, existing)
		replacedClient = existing
	}

	r.clients[client] = struct{}{}
	r.clientsByUser[client.UserID()] = client
	// The first connected user becomes the initial host. After host transfer has
	// happened, reconnecting former hosts must not implicitly reclaim host identity.
	if r.state.HostUserID == "" {
		r.state.HostUserID = client.UserID()
	}
	return JoinResult{
		State:          r.currentStateLocked(r.now()),
		ReplacedClient: replacedClient,
	}
}

// Leave removes a client and reports whether the disconnect triggered host transfer.
func (r *Room) Leave(client *ClientConnection) LeaveResult {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.clients, client)
	if current := r.clientsByUser[client.UserID()]; current == client {
		delete(r.clientsByUser, client.UserID())
	}
	result := LeaveResult{}
	if r.state.HostUserID == client.UserID() {
		previousHost := r.state.HostUserID
		r.state.HostUserID = ""
		for candidate := range r.clients {
			r.state.HostUserID = candidate.UserID()
			break
		}
		if r.state.HostUserID != "" && r.state.HostUserID != previousHost {
			r.state.Seq++
			result.HostTransferred = true
		}
	}

	result.State = r.currentStateLocked(r.now())
	result.Remaining = r.clientsSnapshotLocked()
	result.RoomEmpty = len(r.clients) == 0
	return result
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
	r.state.Ended = false
	r.authorityAt = r.now()
	r.state.Seq++
	return r.currentStateLocked(r.authorityAt), r.clientsSnapshotLocked(), nil
}

// ApplyPlaybackRate updates the room playback rate while preserving a continuous authority timeline.
func (r *Room) ApplyPlaybackRate(
	userID string,
	playbackRate float64,
) (State, []*ClientConnection, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.state.HostUserID != userID {
		return State{}, nil, ErrNotHost
	}

	now := r.now()
	currentState := r.currentStateLocked(now)
	r.state.PositionMs = currentState.PositionMs
	r.state.PlaybackRate = playbackRate
	r.authorityAt = now
	r.state.Seq++
	return r.currentStateLocked(r.authorityAt), r.clientsSnapshotLocked(), nil
}

// ApplyEnded marks the room authority timeline as completed and freezes the position.
func (r *Room) ApplyEnded(userID string, positionMs int64) (State, []*ClientConnection, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.state.HostUserID != userID {
		return State{}, nil, ErrNotHost
	}

	now := r.now()
	currentState := r.currentStateLocked(now)
	frozenPosition := positionMs
	if currentState.PositionMs > frozenPosition {
		frozenPosition = currentState.PositionMs
	}
	r.state.PositionMs = frozenPosition
	r.state.Paused = true
	r.state.Ended = true
	r.authorityAt = now
	r.state.Seq++
	return r.currentStateLocked(r.authorityAt), r.clientsSnapshotLocked(), nil
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
	r.state.Ended = false
	r.state.PositionMs = positionMs
	r.authorityAt = r.now()
	r.state.Seq++
	return r.currentStateLocked(r.authorityAt), r.clientsSnapshotLocked(), nil
}

func (r *Room) clientsSnapshotLocked() []*ClientConnection {
	clients := make([]*ClientConnection, 0, len(r.clients))
	for client := range r.clients {
		clients = append(clients, client)
	}
	return clients
}

func (r *Room) currentStateLocked(now time.Time) State {
	snapshot := r.state
	if snapshot.Paused || snapshot.Ended || r.authorityAt.IsZero() {
		return snapshot
	}

	elapsedMs := now.Sub(r.authorityAt).Milliseconds()
	if elapsedMs <= 0 {
		return snapshot
	}

	progressedMs := int64(math.Round(float64(elapsedMs) * snapshot.PlaybackRate))
	if progressedMs < 0 {
		return snapshot
	}

	snapshot.PositionMs += progressedMs
	return snapshot
}
