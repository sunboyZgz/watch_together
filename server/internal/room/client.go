package room

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/coder/websocket"
	"golang.org/x/sync/semaphore"
)

type ClientConnection struct {
	conn                  *websocket.Conn
	writeMu               *semaphore.Weighted
	outbox                *clientOutbox
	stateMu               sync.RWMutex
	connectionID          string
	userID                string
	roomID                string
	deviceID              string
	lastHeartbeatSentAt   time.Time
	lastHeartbeatAckAt    time.Time
	lastPresenceRefreshAt time.Time
}

type outboundMessage struct {
	message     any
	coalesceKey string
}

const defaultClientOutboxCapacity = 64

func DefaultClientOutboxCapacity() int {
	return defaultClientOutboxCapacity
}

type EnqueueResult struct {
	QueueDepth    int
	QueueCapacity int
	Coalesced     bool
}

type ClientConnectionOptions struct {
	OutboxCapacity int
}

type coalescableOutboundMessage interface {
	OutboxCoalesceKey() string
}

type clientOutbox struct {
	mu       sync.Mutex
	queue    []outboundMessage
	capacity int
	notify   chan struct{}
}

func newClientOutbox(capacity int) *clientOutbox {
	if capacity <= 0 {
		capacity = 1
	}
	return &clientOutbox{
		queue:    make([]outboundMessage, 0, capacity),
		capacity: capacity,
		notify:   make(chan struct{}),
	}
}

func (o *clientOutbox) enqueue(ctx context.Context, message any) (EnqueueResult, error) {
	queued := outboundMessage{
		message:     message,
		coalesceKey: outboxCoalesceKey(message),
	}

	for {
		o.mu.Lock()
		if queued.coalesceKey != "" {
			if o.coalesceLatestLocked(queued) {
				result := EnqueueResult{
					QueueDepth:    len(o.queue),
					QueueCapacity: o.capacity,
					Coalesced:     true,
				}
				o.signalLocked()
				o.mu.Unlock()
				return result, nil
			}
		}
		if len(o.queue) < o.capacity {
			o.queue = append(o.queue, queued)
			result := EnqueueResult{
				QueueDepth:    len(o.queue),
				QueueCapacity: o.capacity,
			}
			o.signalLocked()
			o.mu.Unlock()
			return result, nil
		}
		notify := o.notify
		o.mu.Unlock()

		select {
		case <-ctx.Done():
			return o.snapshot(), ctx.Err()
		case <-notify:
		}
	}
}

func (o *clientOutbox) snapshot() EnqueueResult {
	o.mu.Lock()
	defer o.mu.Unlock()

	return EnqueueResult{
		QueueDepth:    len(o.queue),
		QueueCapacity: o.capacity,
	}
}

func (o *clientOutbox) dequeue(ctx context.Context) (outboundMessage, error) {
	for {
		o.mu.Lock()
		if len(o.queue) > 0 {
			queued := o.queue[0]
			copy(o.queue, o.queue[1:])
			var zero outboundMessage
			o.queue[len(o.queue)-1] = zero
			o.queue = o.queue[:len(o.queue)-1]
			o.signalLocked()
			o.mu.Unlock()
			return queued, nil
		}
		notify := o.notify
		o.mu.Unlock()

		select {
		case <-ctx.Done():
			return outboundMessage{}, ctx.Err()
		case <-notify:
		}
	}
}

func (o *clientOutbox) coalesceLatestLocked(queued outboundMessage) bool {
	for i := len(o.queue) - 1; i >= 0; i-- {
		if o.queue[i].coalesceKey == queued.coalesceKey {
			copy(o.queue[i:], o.queue[i+1:])
			o.queue[len(o.queue)-1] = queued
			return true
		}
	}
	return false
}

func (o *clientOutbox) signalLocked() {
	close(o.notify)
	o.notify = make(chan struct{})
}

func outboxCoalesceKey(message any) string {
	coalescable, ok := message.(coalescableOutboundMessage)
	if !ok {
		return ""
	}
	return coalescable.OutboxCoalesceKey()
}

// NewClientConnection wraps one WebSocket connection with the server-side identity fields we need.
func NewClientConnection(conn *websocket.Conn) *ClientConnection {
	return NewClientConnectionWithOptions(conn, ClientConnectionOptions{})
}

// NewClientConnectionWithOptions wraps one WebSocket connection with explicit runtime limits.
func NewClientConnectionWithOptions(conn *websocket.Conn, options ClientConnectionOptions) *ClientConnection {
	now := time.Now()
	outboxCapacity := options.OutboxCapacity
	if outboxCapacity <= 0 {
		outboxCapacity = defaultClientOutboxCapacity
	}
	return &ClientConnection{
		conn:                  conn,
		writeMu:               semaphore.NewWeighted(1),
		outbox:                newClientOutbox(outboxCapacity),
		connectionID:          newConnectionID(),
		lastHeartbeatSentAt:   now,
		lastHeartbeatAckAt:    now,
		lastPresenceRefreshAt: now,
	}
}

func newConnectionID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("conn-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
}

// SetIdentity stores the logical user and room binding after a successful join_room.
func (c *ClientConnection) SetIdentity(userID string, roomID string) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()

	c.userID = userID
	c.roomID = roomID
	if userID == "" || roomID == "" {
		c.deviceID = ""
	}
}

// SetDeviceID stores the client-persistent device identifier after join_room validation.
func (c *ClientConnection) SetDeviceID(deviceID string) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()

	c.deviceID = deviceID
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

// DeviceID returns the client-persistent device identifier bound to this connection.
func (c *ClientConnection) DeviceID() string {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()

	return c.deviceID
}

// ConnectionID is the process-local session identifier used for distributed
// active-device ownership checks and release safety.
func (c *ClientConnection) ConnectionID() string {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()

	return c.connectionID
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

// EnqueueJSON adds one message to this connection's outbound queue.
func (c *ClientConnection) EnqueueJSON(ctx context.Context, message any) (EnqueueResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if c.outbox == nil {
		return EnqueueResult{}, c.WriteJSON(ctx, message)
	}

	return c.outbox.enqueue(ctx, message)
}

// RunWriteLoop drains queued outbound messages and writes them to the websocket in order.
func (c *ClientConnection) RunWriteLoop(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if c.outbox == nil {
		<-ctx.Done()
		return ctx.Err()
	}

	for {
		queued, err := c.outbox.dequeue(ctx)
		if err != nil {
			return err
		}
		if err := c.WriteJSON(ctx, queued.message); err != nil {
			return err
		}
	}
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

func (c *ClientConnection) PresenceRefreshDue(now time.Time, interval time.Duration) bool {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()

	if interval <= 0 {
		return true
	}
	return now.Sub(c.lastPresenceRefreshAt) >= interval
}

func (c *ClientConnection) MarkPresenceRefreshed(refreshedAt time.Time) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()

	c.lastPresenceRefreshAt = refreshedAt
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
