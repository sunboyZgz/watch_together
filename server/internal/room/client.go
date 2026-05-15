package room

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/coder/websocket"
	"golang.org/x/sync/semaphore"
)

type ClientConnection struct {
	conn                *websocket.Conn
	writeMu             *semaphore.Weighted
	stateMu             sync.RWMutex
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
		writeMu:             semaphore.NewWeighted(1),
		lastHeartbeatSentAt: now,
		lastHeartbeatAckAt:  now,
	}
}

// SetIdentity stores the logical user and room binding after a successful join_room.
func (c *ClientConnection) SetIdentity(userID string, roomID string) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()

	c.userID = userID
	c.roomID = roomID
}

// UserID returns the logical user bound to this connection.
func (c *ClientConnection) UserID() string {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()

	return c.userID
}

// RoomID returns the room currently associated with this connection.
func (c *ClientConnection) RoomID() string {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()

	return c.roomID
}

// WriteJSON serializes one protocol message and writes it as a text WebSocket frame.
func (c *ClientConnection) WriteJSON(ctx context.Context, message any) error {
	if ctx == nil {
		ctx = context.Background()
	}

	data, err := json.Marshal(message)
	if err != nil {
		return err
	}

	if err := c.writeMu.Acquire(ctx, 1); err != nil {
		return err
	}
	defer c.writeMu.Release(1)

	// The write lock keeps concurrent responses from interleaving on the same socket.
	return c.conn.Write(ctx, websocket.MessageText, data)
}

// MarkHeartbeatSent records the last outbound heartbeat time.
func (c *ClientConnection) MarkHeartbeatSent(sentAt time.Time) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()

	c.lastHeartbeatSentAt = sentAt
}

// MarkHeartbeatAck records the most recent heartbeat_ack time.
func (c *ClientConnection) MarkHeartbeatAck(ackAt time.Time) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()

	c.lastHeartbeatAckAt = ackAt
}

// HeartbeatTimedOut reports whether the client has gone past the allowed ack window.
func (c *ClientConnection) HeartbeatTimedOut(now time.Time, timeout time.Duration) bool {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()

	return now.Sub(c.lastHeartbeatAckAt) > timeout
}

// Close terminates the underlying WebSocket connection.
func (c *ClientConnection) Close(status websocket.StatusCode, reason string) error {
	return c.conn.Close(status, reason)
}

// CloseNow force-closes the underlying WebSocket connection without waiting for a close handshake.
func (c *ClientConnection) CloseNow() error {
	if c.conn == nil {
		return nil
	}
	return c.conn.CloseNow()
}
