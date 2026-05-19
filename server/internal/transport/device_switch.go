package transport

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"watch_together/server/internal/room"
)

const defaultRoomDeviceSwitchTimeout = 30 * time.Second

type roomDeviceSwitchRegistry struct {
	mu        sync.Mutex
	pending   map[string]roomDeviceSwitch
	byPending map[*room.ClientConnection]string
	byActive  map[*room.ClientConnection]map[string]struct{}
}

type roomDeviceSwitch struct {
	RequestID     string
	UserID        string
	TargetRoomID  string
	ActiveRoomID  string
	ActiveClient  *room.ClientConnection
	PendingClient *room.ClientConnection
	ExpiresAt     time.Time
}

func newRoomDeviceSwitchRegistry() *roomDeviceSwitchRegistry {
	return &roomDeviceSwitchRegistry{
		pending:   make(map[string]roomDeviceSwitch),
		byPending: make(map[*room.ClientConnection]string),
		byActive:  make(map[*room.ClientConnection]map[string]struct{}),
	}
}

// 创建切换请求
func (r *roomDeviceSwitchRegistry) create(
	userID string,
	targetRoomID string,
	activeRoomID string,
	activeClient *room.ClientConnection,
	pendingClient *room.ClientConnection,
	expiresAt time.Time,
) (roomDeviceSwitch, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if requestID, ok := r.byPending[pendingClient]; ok {
		return r.pending[requestID], false
	}
	requestID := generateRoomDeviceSwitchRequestID()
	pending := roomDeviceSwitch{
		RequestID:     requestID,
		UserID:        userID,
		TargetRoomID:  targetRoomID,
		ActiveRoomID:  activeRoomID,
		ActiveClient:  activeClient,
		PendingClient: pendingClient,
		ExpiresAt:     expiresAt,
	}
	r.pending[requestID] = pending
	r.byPending[pendingClient] = requestID
	if r.byActive[activeClient] == nil {
		r.byActive[activeClient] = make(map[string]struct{})
	}
	r.byActive[activeClient][requestID] = struct{}{}
	return pending, true
}

// 取出并删除请求
func (r *roomDeviceSwitchRegistry) take(requestID string) (roomDeviceSwitch, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	pending, ok := r.pending[requestID]
	if !ok {
		return roomDeviceSwitch{}, false
	}
	r.deleteLocked(pending)
	return pending, true
}

func (r *roomDeviceSwitchRegistry) get(requestID string) (roomDeviceSwitch, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	pending, ok := r.pending[requestID]
	return pending, ok
}

// 取消某个 client 相关的所有请求
func (r *roomDeviceSwitchRegistry) cancelForClient(client *room.ClientConnection) []roomDeviceSwitch {
	r.mu.Lock()
	defer r.mu.Unlock()

	var canceled []roomDeviceSwitch
	if requestID, ok := r.byPending[client]; ok {
		if pending, exists := r.pending[requestID]; exists {
			canceled = append(canceled, pending)
			r.deleteLocked(pending)
		}
	}
	for requestID := range r.byActive[client] {
		if pending, exists := r.pending[requestID]; exists {
			canceled = append(canceled, pending)
			r.deleteLocked(pending)
		}
	}
	return canceled
}

func (r *roomDeviceSwitchRegistry) deleteLocked(pending roomDeviceSwitch) {
	delete(r.pending, pending.RequestID)
	delete(r.byPending, pending.PendingClient)
	if activeRequests := r.byActive[pending.ActiveClient]; activeRequests != nil {
		delete(activeRequests, pending.RequestID)
		if len(activeRequests) == 0 {
			delete(r.byActive, pending.ActiveClient)
		}
	}
}

func generateRoomDeviceSwitchRequestID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(bytes[:])
}
