package room

import "sync"

type Manager struct {
	mu    sync.RWMutex
	rooms map[string]*Room
}

// NewManager creates the top-level in-memory registry for all active rooms.
func NewManager() *Manager {
	return &Manager{
		rooms: make(map[string]*Room),
	}
}

// GetOrCreate returns an existing room or creates a new one on first join.
func (m *Manager) GetOrCreate(roomID string) *Room {
	m.mu.Lock()
	defer m.mu.Unlock()

	if room, ok := m.rooms[roomID]; ok {
		return room
	}

	room := New(roomID)
	m.rooms[roomID] = room
	return room
}

// RemoveClient removes a client from its room and prunes empty rooms.
func (m *Manager) RemoveClient(client *ClientConnection) {
	roomID := client.RoomID()
	if roomID == "" {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	room, ok := m.rooms[roomID]
	if !ok {
		return
	}

	room.Leave(client)
	if room.ClientCount() == 0 {
		delete(m.rooms, roomID)
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
