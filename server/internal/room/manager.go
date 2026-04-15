package room

import (
	"crypto/rand"
	"errors"
	"sync"
)

var ErrUnableToGenerateRoomID = errors.New("unable to generate unique room id")

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

// CreateRoom generates a unique room ID, initializes the room state, and registers it in memory.
func (m *Manager) CreateRoom(hostUserID string, mediaID string) (*Room, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

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

// Get returns one room by ID without creating a new one.
func (m *Manager) Get(roomID string) (*Room, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	room, ok := m.rooms[roomID]
	return room, ok
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
