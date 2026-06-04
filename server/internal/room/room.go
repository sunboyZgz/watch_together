package room

import (
	"errors"
	"sync"
	"time"

	"watch_together/server/internal/realtime"
)

var (
	ErrNotHost             = errors.New("only host can control playback")
	ErrRoomFull            = errors.New("room is full")
	ErrSeqMismatch         = errors.New("control seq does not match current room state")
	ErrNotActiveRoomDevice = errors.New("not active room device")
)

const (
	reasonMediaChange = "media_change"
	reasonMediaEnd    = "media_end"
	reasonHostLeft    = "host_left"
	reasonHostRejoin  = "host_rejoin"
)

type State struct {
	RoomID          string
	MediaID         string
	MediaDurationMs *int64
	HostUserID      string
	Paused          bool
	Ended           bool
	PositionMs      int64
	Velocity        float64
	ServerTimeMs    int64
	PlaybackRate    float64
	Seq             int64
	Reason          string
}

type Room struct {
	id            string
	mu            sync.RWMutex
	clients       map[*ClientConnection]struct{}
	clientsByUser map[string]*ClientConnection
	ownerUserID   string
	state         State
	now           func() time.Time
}

type LeaveResult struct {
	State           State
	Remaining       []*ClientConnection
	HostUnavailable bool
	RoomEmpty       bool
}

type JoinResult struct {
	State             State
	Clients           []*ClientConnection
	ReplacedClient    *ClientConnection
	MembershipChanged bool
	HostReclaimed     bool
	Err               error
}

type SwitchClientResult struct {
	State   State
	Clients []*ClientConnection
	Err     error
}

// New creates a room with a minimal default playback state.
func New(id string) *Room {
	return newWithClock(id, time.Now)
}

func newWithClock(id string, now func() time.Time) *Room {
	vector := realtime.NewTimelineVector(now())
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
			PositionMs:   vector.PositionMs,
			Velocity:     vector.Velocity,
			ServerTimeMs: vector.ServerTimeMs,
			PlaybackRate: 1.0,
			Seq:          vector.Seq,
			Reason:       vector.Reason,
		},
		now: now,
	}
}

// NewCreatedRoom creates a room that already has an initial host and media binding.
func NewCreatedRoom(id string, hostUserID string, mediaID string) *Room {
	return NewCreatedRoomWithMedia(id, hostUserID, mediaID, nil)
}

// NewCreatedRoomWithMedia creates a room with initial host and media timing metadata.
func NewCreatedRoomWithMedia(id string, hostUserID string, mediaID string, mediaDurationMs *int64) *Room {
	room := New(id)
	room.state.MediaID = mediaID
	room.state.MediaDurationMs = cloneDurationMs(mediaDurationMs)
	room.stateFromVectorLocked(
		realtime.NewTimelineVectorWithBounds(room.now(), room.timelineBoundsLocked()),
		room.state.PlaybackRate,
		room.state.Ended,
	)
	room.state.HostUserID = hostUserID
	room.ownerUserID = hostUserID
	return room
}

// ID returns the stable room identifier.
func (r *Room) ID() string {
	return r.id
}

// BindMedia attaches media metadata without changing the current timeline when the media is unchanged.
func (r *Room) BindMedia(mediaID string, mediaDurationMs *int64) State {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.state.MediaID != "" && r.state.MediaID != mediaID {
		nextSeq := r.state.Seq + 1
		r.state.MediaID = mediaID
		r.state.MediaDurationMs = cloneDurationMs(mediaDurationMs)
		next := realtime.NewTimelineVectorWithBounds(r.now(), r.timelineBoundsLocked())
		next.Seq = nextSeq
		next.Reason = reasonMediaChange
		r.stateFromVectorLocked(next, r.state.PlaybackRate, false)
		return r.currentStateLocked(r.now())
	}

	r.state.MediaID = mediaID
	if mediaDurationMs != nil {
		r.state.MediaDurationMs = cloneDurationMs(mediaDurationMs)
	}
	return r.currentStateLocked(r.now())
}

// StateSnapshot returns the current room state in a read-safe way.
func (r *Room) StateSnapshot() State {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.currentStateLocked(r.now())
}

// Join registers a client in the room and replaces any previous connection from the same user.
func (r *Room) Join(client *ClientConnection) JoinResult {
	return r.JoinWithLimit(client, 0)
}

// JoinWithLimit registers a client while enforcing an optional maximum room size.
func (r *Room) JoinWithLimit(client *ClientConnection, maxClients int) JoinResult {
	r.mu.Lock()
	defer r.mu.Unlock()

	var replacedClient *ClientConnection
	membershipChanged := false
	previousHostUserID := r.state.HostUserID
	if existing := r.clientsByUser[client.UserID()]; existing != nil && existing != client {
		delete(r.clients, existing)
		replacedClient = existing
	} else if r.clientsByUser[client.UserID()] == nil {
		if maxClients > 0 && len(r.clients) >= maxClients {
			return JoinResult{
				State: r.currentStateLocked(r.now()),
				Err:   ErrRoomFull,
			}
		}
		membershipChanged = true
	}

	r.clients[client] = struct{}{}
	r.clientsByUser[client.UserID()] = client
	if r.state.HostUserID == "" && r.canClaimHostLocked(client.UserID()) {
		if r.ownerUserID != "" && previousHostUserID != client.UserID() {
			now := r.now()
			next := r.vectorLocked().SnapshotAt(now)
			next.Seq = r.state.Seq + 1
			next.Reason = reasonHostRejoin
			r.stateFromVectorLocked(next, r.state.PlaybackRate, r.state.Ended)
		}
		r.state.HostUserID = client.UserID()
	}
	return JoinResult{
		State:             r.currentStateLocked(r.now()),
		Clients:           r.clientsSnapshotLocked(),
		ReplacedClient:    replacedClient,
		MembershipChanged: membershipChanged,
		HostReclaimed:     previousHostUserID == "" && r.state.HostUserID == r.ownerUserID && r.state.HostUserID != "",
	}
}

// SwitchClient replaces the active connection for the same logical user without
// changing room membership or the authoritative playback timeline.
func (r *Room) SwitchClient(activeClient *ClientConnection, nextClient *ClientConnection) SwitchClientResult {
	r.mu.Lock()
	defer r.mu.Unlock()

	if activeClient == nil || nextClient == nil || activeClient.UserID() == "" ||
		activeClient.UserID() != nextClient.UserID() {
		return SwitchClientResult{
			State: r.currentStateLocked(r.now()),
			Err:   ErrNotActiveRoomDevice,
		}
	}
	if current := r.clientsByUser[activeClient.UserID()]; current != activeClient {
		return SwitchClientResult{
			State: r.currentStateLocked(r.now()),
			Err:   ErrNotActiveRoomDevice,
		}
	}

	delete(r.clients, activeClient)
	r.clients[nextClient] = struct{}{}
	r.clientsByUser[nextClient.UserID()] = nextClient
	return SwitchClientResult{
		State:   r.currentStateLocked(r.now()),
		Clients: r.clientsSnapshotLocked(),
	}
}

// ActiveClientForUser returns the current active room connection for a logical user.
func (r *Room) ActiveClientForUser(userID string) (*ClientConnection, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	client, ok := r.clientsByUser[userID]
	return client, ok
}

// IsActiveClient reports whether the connection is the authoritative room device
// for its logical user.
func (r *Room) IsActiveClient(client *ClientConnection) bool {
	if client == nil {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.clientsByUser[client.UserID()] == client
}

// Leave removes a client and pauses playback if the current host disconnects.
func (r *Room) Leave(client *ClientConnection) LeaveResult {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, wasPresent := r.clients[client]
	delete(r.clients, client)
	if current := r.clientsByUser[client.UserID()]; current == client {
		delete(r.clientsByUser, client.UserID())
	}
	result := LeaveResult{}
	if wasPresent && r.state.HostUserID == client.UserID() {
		now := r.now()
		next := realtime.Pause(r.vectorLocked(), now)
		next.Reason = reasonHostLeft
		r.stateFromVectorLocked(next, r.state.PlaybackRate, r.state.Ended)
		r.state.HostUserID = ""
		result.HostUnavailable = true
	}

	result.State = r.currentStateLocked(r.now())
	result.Remaining = r.clientsSnapshotLocked()
	result.RoomEmpty = len(r.clients) == 0
	return result
}

func (r *Room) canClaimHostLocked(userID string) bool {
	if userID == "" {
		return false
	}
	if r.ownerUserID == "" {
		return true
	}
	return userID == r.ownerUserID
}

// ClientCount reports how many active connections the room currently holds.
func (r *Room) ClientCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.clients)
}

// ClientsSnapshot returns the current local WebSocket clients without exposing
// the room lock to callers.
func (r *Room) ClientsSnapshot() []*ClientConnection {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.clientsSnapshotLocked()
}

// ApplyPlay updates the room's authority state for a play action and snapshots the
// active clients so the transport layer can broadcast without holding the room lock.
func (r *Room) ApplyPlay(userID string, positionMs int64) (State, []*ClientConnection, error) {
	return r.applyPlay(userID, positionMs, nil)
}

func (r *Room) ApplyPlayIfSeq(userID string, positionMs int64, expectedSeq int64) (State, []*ClientConnection, error) {
	return r.applyPlay(userID, positionMs, &expectedSeq)
}

func (r *Room) applyPlay(userID string, positionMs int64, expectedSeq *int64) (State, []*ClientConnection, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.state.HostUserID != userID {
		return State{}, nil, ErrNotHost
	}
	if expectedSeq != nil && *expectedSeq != r.state.Seq {
		return r.currentStateLocked(r.now()), nil, ErrSeqMismatch
	}

	now := r.now()
	playbackRate := r.state.PlaybackRate
	if playbackRate <= 0 {
		playbackRate = 1.0
	}
	next := realtime.Play(r.vectorLocked(), now, playbackRate)
	if isAtEnd(next) {
		next.Velocity = 0
	}
	r.stateFromVectorLocked(next, playbackRate, isAtEnd(next))
	return r.currentStateLocked(now), r.clientsSnapshotLocked(), nil
}

// ApplyPause updates the room's authority state for a pause action.
func (r *Room) ApplyPause(userID string, positionMs int64) (State, []*ClientConnection, error) {
	return r.applyPause(userID, positionMs, nil)
}

func (r *Room) ApplyPauseIfSeq(userID string, positionMs int64, expectedSeq int64) (State, []*ClientConnection, error) {
	return r.applyPause(userID, positionMs, &expectedSeq)
}

func (r *Room) applyPause(userID string, positionMs int64, expectedSeq *int64) (State, []*ClientConnection, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.state.HostUserID != userID {
		return State{}, nil, ErrNotHost
	}
	if expectedSeq != nil && *expectedSeq != r.state.Seq {
		return r.currentStateLocked(r.now()), nil, ErrSeqMismatch
	}

	now := r.now()
	next := realtime.Pause(r.vectorLocked(), now)
	r.stateFromVectorLocked(next, r.state.PlaybackRate, isAtEnd(next))
	return r.currentStateLocked(now), r.clientsSnapshotLocked(), nil
}

// ApplySeek updates the room position while preserving the current paused flag.
func (r *Room) ApplySeek(userID string, positionMs int64) (State, []*ClientConnection, error) {
	return r.applySeek(userID, positionMs, nil)
}

func (r *Room) ApplySeekIfSeq(userID string, positionMs int64, expectedSeq int64) (State, []*ClientConnection, error) {
	return r.applySeek(userID, positionMs, &expectedSeq)
}

func (r *Room) applySeek(userID string, positionMs int64, expectedSeq *int64) (State, []*ClientConnection, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.state.HostUserID != userID {
		return State{}, nil, ErrNotHost
	}
	if expectedSeq != nil && *expectedSeq != r.state.Seq {
		return r.currentStateLocked(r.now()), nil, ErrSeqMismatch
	}

	now := r.now()
	next := realtime.Seek(r.vectorLocked(), now, positionMs)
	r.stateFromVectorLocked(next, r.state.PlaybackRate, isAtEnd(next))
	return r.currentStateLocked(now), r.clientsSnapshotLocked(), nil
}

// ApplyPlaybackRate updates the room playback rate while preserving a continuous authority timeline.
func (r *Room) ApplyPlaybackRate(
	userID string,
	playbackRate float64,
) (State, []*ClientConnection, error) {
	return r.applyPlaybackRate(userID, playbackRate, nil)
}

func (r *Room) ApplyPlaybackRateIfSeq(
	userID string,
	playbackRate float64,
	expectedSeq int64,
) (State, []*ClientConnection, error) {
	return r.applyPlaybackRate(userID, playbackRate, &expectedSeq)
}

func (r *Room) applyPlaybackRate(
	userID string,
	playbackRate float64,
	expectedSeq *int64,
) (State, []*ClientConnection, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.state.HostUserID != userID {
		return State{}, nil, ErrNotHost
	}
	if expectedSeq != nil && *expectedSeq != r.state.Seq {
		return r.currentStateLocked(r.now()), nil, ErrSeqMismatch
	}

	now := r.now()
	previous := r.vectorLocked()
	next := realtime.RateChange(previous, now, playbackRate)
	if previous.Velocity == 0 {
		next.Velocity = 0
	}
	if isAtEnd(next) {
		next.Velocity = 0
	}
	r.state.PlaybackRate = playbackRate
	r.stateFromVectorLocked(next, playbackRate, r.state.Ended || isAtEnd(next))
	return r.currentStateLocked(now), r.clientsSnapshotLocked(), nil
}

// ApplyEnded marks the room authority timeline as completed and freezes the position.
func (r *Room) ApplyEnded(userID string, positionMs int64) (State, []*ClientConnection, error) {
	return r.applyEnded(userID, positionMs, nil)
}

func (r *Room) ApplyEndedIfSeq(userID string, positionMs int64, expectedSeq int64) (State, []*ClientConnection, error) {
	return r.applyEnded(userID, positionMs, &expectedSeq)
}

func (r *Room) applyEnded(userID string, positionMs int64, expectedSeq *int64) (State, []*ClientConnection, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.state.HostUserID != userID {
		return State{}, nil, ErrNotHost
	}
	if expectedSeq != nil && *expectedSeq != r.state.Seq {
		return r.currentStateLocked(r.now()), nil, ErrSeqMismatch
	}

	now := r.now()
	next := realtime.StopAt(r.vectorLocked(), now, positionMs, reasonMediaEnd)
	r.stateFromVectorLocked(next, r.state.PlaybackRate, true)
	return r.currentStateLocked(now), r.clientsSnapshotLocked(), nil
}

func (r *Room) clientsSnapshotLocked() []*ClientConnection {
	clients := make([]*ClientConnection, 0, len(r.clients))
	for client := range r.clients {
		clients = append(clients, client)
	}
	return clients
}

// save snapshot of current state with position updated to now, without modifying the actual room state
func (r *Room) currentStateLocked(now time.Time) State {
	snapshot := r.state
	vector := r.vectorLocked().SnapshotAt(now)
	snapshot.PositionMs = vector.PositionMs
	snapshot.ServerTimeMs = vector.ServerTimeMs
	snapshot.Paused = vector.Velocity == 0
	if isAtEnd(vector) {
		snapshot.Ended = true
		snapshot.Paused = true
		snapshot.Velocity = 0
	}
	return snapshot
}

func (r *Room) vectorLocked() realtime.TimelineVector {
	return realtime.TimelineVector{
		PositionMs:   r.state.PositionMs,
		Velocity:     r.state.Velocity,
		ServerTimeMs: r.state.ServerTimeMs,
		Seq:          r.state.Seq,
		Reason:       r.state.Reason,
		Bounds:       r.timelineBoundsLocked(),
	}
}

func (r *Room) stateFromVectorLocked(vector realtime.TimelineVector, playbackRate float64, ended bool) {
	r.state.PositionMs = vector.PositionMs
	r.state.Velocity = vector.Velocity
	r.state.ServerTimeMs = vector.ServerTimeMs
	r.state.Seq = vector.Seq
	r.state.Reason = vector.Reason
	r.state.Paused = vector.Velocity == 0
	r.state.Ended = ended
	if playbackRate <= 0 {
		playbackRate = 1.0
	}
	r.state.PlaybackRate = playbackRate
}

func (r *Room) timelineBoundsLocked() *realtime.TimelineBounds {
	if r.state.MediaDurationMs == nil || *r.state.MediaDurationMs < 0 {
		return nil
	}
	return &realtime.TimelineBounds{
		StartMs: 0,
		EndMs:   r.state.MediaDurationMs,
	}
}

func isAtEnd(vector realtime.TimelineVector) bool {
	if vector.Bounds == nil || vector.Bounds.EndMs == nil {
		return false
	}
	return vector.PositionMs >= *vector.Bounds.EndMs
}

func cloneDurationMs(durationMs *int64) *int64 {
	if durationMs == nil {
		return nil
	}
	cloned := *durationMs
	return &cloned
}
