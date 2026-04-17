package room

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/coder/websocket"
)

type ClientConnection struct {
	conn                *websocket.Conn
	mu                  sync.Mutex
	userID              string
	roomID              string
	lastHeartbeatSentAt time.Time
	lastHeartbeatAckAt  time.Time
}

// NewClientConnection wraps one WebSocket connection with the server-side identity fields we need.
func NewClientConnection(conn *websocket.Conn) *ClientConnection {
	now := time.Now()
	return &ClientConnection{
		conn:                conn,
		lastHeartbeatSentAt: now,
		lastHeartbeatAckAt:  now,
	}
}

// SetIdentity stores the logical user and room binding after a successful join_room.
func (c *ClientConnection) SetIdentity(userID string, roomID string) {
	c.userID = userID
	c.roomID = roomID
}

// UserID returns the logical user bound to this connection.
func (c *ClientConnection) UserID() string {
	return c.userID
}

// RoomID returns the room currently associated with this connection.
func (c *ClientConnection) RoomID() string {
	return c.roomID
}

// WriteJSON serializes one protocol message and writes it as a text WebSocket frame.
func (c *ClientConnection) WriteJSON(ctx context.Context, message any) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// The write lock keeps concurrent responses from interleaving on the same socket.
	return c.conn.Write(ctx, websocket.MessageText, data)
}

// MarkHeartbeatSent records the last outbound heartbeat time.
func (c *ClientConnection) MarkHeartbeatSent(sentAt time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.lastHeartbeatSentAt = sentAt
}

// MarkHeartbeatAck records the most recent heartbeat_ack time.
func (c *ClientConnection) MarkHeartbeatAck(ackAt time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.lastHeartbeatAckAt = ackAt
}

// HeartbeatTimedOut reports whether the client has gone past the allowed ack window.
func (c *ClientConnection) HeartbeatTimedOut(now time.Time, timeout time.Duration) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return now.Sub(c.lastHeartbeatAckAt) > timeout
}

// Close terminates the underlying WebSocket connection.
func (c *ClientConnection) Close(status websocket.StatusCode, reason string) error {
	return c.conn.Close(status, reason)
}
