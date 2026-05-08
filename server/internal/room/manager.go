package room

import (
	"context"
	"crypto/rand"
	"errors"
	"sync"
	"time"
)

var ErrUnableToGenerateRoomID = errors.New("unable to generate unique room id")

const (
	defaultEmptyRoomGracePeriod = 2 * time.Minute
	defaultCleanupInterval      = 5 * time.Second
)

func DefaultCleanupInterval() time.Duration {
	return defaultCleanupInterval
}

type Manager struct {
	mu                   sync.RWMutex
	rooms                map[string]*Room
	emptySince           map[string]time.Time
	now                  func() time.Time
	emptyRoomGracePeriod time.Duration
	hooks                LifecycleHooks
}

type LifecycleHooks struct {
	OnRoomBecameEmpty func(roomID string, emptySince time.Time, destroyAfter time.Time)
	OnRoomReactivated func(roomID string)
	OnRoomDestroyed   func(roomID string)
}

type RemoveClientResult struct {
	State           State
	Remaining       []*ClientConnection
	HostTransferred bool
	RoomRemoved     bool
}

// NewManager creates the top-level in-memory registry for all active rooms.
func NewManager() *Manager {
	return newManagerWithClock(time.Now, defaultEmptyRoomGracePeriod)
}

func newManagerWithClock(now func() time.Time, emptyRoomGracePeriod time.Duration) *Manager {
	return &Manager{
		rooms:                make(map[string]*Room),
		emptySince:           make(map[string]time.Time),
		now:                  now,
		emptyRoomGracePeriod: emptyRoomGracePeriod,
	}
}

// CreateRoom generates a unique room ID, initializes the room state, and registers it in memory.
func (m *Manager) CreateRoom(hostUserID string, mediaID string) (*Room, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, _ = m.cleanupExpiredRoomsLocked(m.now())

	for range 10 {
		roomID, err := generateRoomID(6)
		if err != nil {
			return nil, err
		}
		if _, exists := m.rooms[roomID]; exists {
			continue
		}

		room := NewCreatedRoom(roomID, hostUserID, mediaID)
		m.rooms[roomID] = room
		return room, nil
	}

	return nil, ErrUnableToGenerateRoomID
}

// RegisterCreatedRoom mirrors a persistent room into the in-memory sync registry.
func (m *Manager) RegisterCreatedRoom(roomID string, hostUserID string, mediaID string) *Room {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, _ = m.cleanupExpiredRoomsLocked(m.now())

	if existing, ok := m.rooms[roomID]; ok {
		return existing
	}
	registeredRoom := NewCreatedRoom(roomID, hostUserID, mediaID)
	m.rooms[roomID] = registeredRoom
	return registeredRoom
}

// GetOrCreate returns an existing room or creates a new one on first join.
func (m *Manager) GetOrCreate(roomID string) *Room {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, _ = m.cleanupExpiredRoomsLocked(m.now())

	if room, ok := m.rooms[roomID]; ok {
		return room
	}

	room := New(roomID)
	m.rooms[roomID] = room
	return room
}

// RemoveClient removes a client from its room, prunes empty rooms, and reports
// whether the disconnect immediately transferred host ownership.
func (m *Manager) RemoveClient(client *ClientConnection) RemoveClientResult {
	roomID := client.RoomID()
	if roomID == "" {
		return RemoveClientResult{}
	}

	m.mu.Lock()

	room, ok := m.rooms[roomID]
	if !ok {
		m.mu.Unlock()
		return RemoveClientResult{}
	}

	leaveResult := room.Leave(client)
	var emptySince time.Time
	var destroyAfter time.Time
	triggerEmptyHook := false
	result := RemoveClientResult{
		State:           leaveResult.State,
		Remaining:       leaveResult.Remaining,
		HostTransferred: leaveResult.HostTransferred,
	}
	if leaveResult.RoomEmpty {
		if _, existed := m.emptySince[roomID]; !existed {
			emptySince = m.now()
			destroyAfter = emptySince.Add(m.emptyRoomGracePeriod)
			m.emptySince[roomID] = emptySince
			triggerEmptyHook = true
		}
	}
	m.mu.Unlock()
	if triggerEmptyHook && m.hooks.OnRoomBecameEmpty != nil {
		m.hooks.OnRoomBecameEmpty(roomID, emptySince, destroyAfter)
	}
	return result
}

// MarkRoomActive clears any pending empty-room cleanup after a member rejoins.
func (m *Manager) MarkRoomActive(roomID string) {
	m.mu.Lock()
	_, existed := m.emptySince[roomID]

	delete(m.emptySince, roomID)
	m.mu.Unlock()
	if existed && m.hooks.OnRoomReactivated != nil {
		m.hooks.OnRoomReactivated(roomID)
	}
}

// CleanupExpiredRooms removes rooms whose empty grace period has elapsed.
func (m *Manager) CleanupExpiredRooms() int {
	m.mu.Lock()
	removedCount, removedRoomIDs := m.cleanupExpiredRoomsLocked(m.now())
	m.mu.Unlock()
	if m.hooks.OnRoomDestroyed != nil {
		for _, roomID := range removedRoomIDs {
			m.hooks.OnRoomDestroyed(roomID)
		}
	}
	return removedCount
}

// StartCleanupLoop periodically removes expired empty rooms.
func (m *Manager) StartCleanupLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.CleanupExpiredRooms()
		}
	}
}

// RoomCount exposes the current number of active rooms for tests and diagnostics.
func (m *Manager) RoomCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.rooms)
}

// ClientCount returns the number of clients currently tracked in one room.
func (m *Manager) ClientCount(roomID string) int {
	m.mu.RLock()
	room, ok := m.rooms[roomID]
	m.mu.RUnlock()
	if !ok {
		return 0
	}
	return room.ClientCount()
}

// Get returns one room by ID without creating a new one.
func (m *Manager) Get(roomID string) (*Room, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, _ = m.cleanupExpiredRoomsLocked(m.now())

	room, ok := m.rooms[roomID]
	return room, ok
}

func (m *Manager) SetLifecycleHooks(hooks LifecycleHooks) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hooks = hooks
}

func (m *Manager) cleanupExpiredRoomsLocked(now time.Time) (int, []string) {
	removed := 0
	removedRoomIDs := make([]string, 0)
	for roomID, emptySince := range m.emptySince {
		if now.Sub(emptySince) < m.emptyRoomGracePeriod {
			continue
		}
		delete(m.emptySince, roomID)
		if _, ok := m.rooms[roomID]; ok {
			delete(m.rooms, roomID)
			removed++
			removedRoomIDs = append(removedRoomIDs, roomID)
		}
	}
	return removed, removedRoomIDs
}

func generateRoomID(length int) (string, error) {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	for i := range bytes {
		bytes[i] = chars[int(bytes[i])%len(chars)]
	}
	return string(bytes), nil
}
