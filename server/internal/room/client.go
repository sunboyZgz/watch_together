package room

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/coder/websocket"
)

type ClientConnection struct {
	conn   *websocket.Conn
	mu     sync.Mutex
	userID string
	roomID string
}

// NewClientConnection wraps one WebSocket connection with the server-side identity fields we need.
func NewClientConnection(conn *websocket.Conn) *ClientConnection {
	return &ClientConnection{conn: conn}
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

// Close terminates the underlying WebSocket connection.
func (c *ClientConnection) Close(status websocket.StatusCode, reason string) error {
	return c.conn.Close(status, reason)
}
