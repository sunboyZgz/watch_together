package eventbus

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

const (
	EventBusNATSCore            = "nats_core"
	DefaultNATSURL              = "nats://127.0.0.1:4222"
	DefaultRoomBroadcastSubject = "wt.room.broadcast.v1"
	DefaultRoomControlSubject   = "wt.room.control.v1"
	DefaultNATSName             = "watch-together-roomserver"
)

var ErrEventBusDisabled = errors.New("event bus is disabled")

type RoomBroadcastEvent struct {
	InstanceID    string          `json:"instanceId"`
	RoomID        string          `json:"roomId"`
	Type          string          `json:"type"`
	Payload       json.RawMessage `json:"payload"`
	Seq           int64           `json:"seq,omitempty"`
	PublishedAtMs int64           `json:"publishedAtMs"`
}

type RoomBroadcastHandler func(ctx context.Context, event RoomBroadcastEvent)

type RoomBroadcastBus interface {
	PublishRoomEnvelope(ctx context.Context, event RoomBroadcastEvent) error
	SubscribeRoomBroadcasts(ctx context.Context, handler RoomBroadcastHandler) error
	Close() error
}

type NATSConfig struct {
	URL            string
	Name           string
	Subject        string
	ControlSubject string
}

func NormalizeEventBus(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return EventBusNATSCore, nil
	}
	if normalized != EventBusNATSCore {
		return "", errors.New("unsupported WS_EVENT_BUS " + value)
	}
	return normalized, nil
}

func NormalizeNATSConfig(config NATSConfig) NATSConfig {
	config.URL = strings.TrimSpace(config.URL)
	if config.URL == "" {
		config.URL = DefaultNATSURL
	}
	config.Name = strings.TrimSpace(config.Name)
	if config.Name == "" {
		config.Name = DefaultNATSName
	}
	config.Subject = strings.TrimSpace(config.Subject)
	if config.Subject == "" {
		config.Subject = DefaultRoomBroadcastSubject
	}
	config.ControlSubject = strings.TrimSpace(config.ControlSubject)
	if config.ControlSubject == "" {
		config.ControlSubject = DefaultRoomControlSubject
	}
	return config
}

func EncodeRoomBroadcastEvent(event RoomBroadcastEvent) ([]byte, error) {
	return json.Marshal(event)
}

func DecodeRoomBroadcastEvent(data []byte) (RoomBroadcastEvent, error) {
	var event RoomBroadcastEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return RoomBroadcastEvent{}, err
	}
	return event, nil
}

type DisabledRoomBroadcastBus struct{}

func NewDisabledRoomBroadcastBus() DisabledRoomBroadcastBus {
	return DisabledRoomBroadcastBus{}
}

func (DisabledRoomBroadcastBus) PublishRoomEnvelope(context.Context, RoomBroadcastEvent) error {
	return nil
}

func (DisabledRoomBroadcastBus) SubscribeRoomBroadcasts(context.Context, RoomBroadcastHandler) error {
	return nil
}

func (DisabledRoomBroadcastBus) Close() error {
	return nil
}

type MemoryRoomBroadcastBus struct {
	mu       sync.Mutex
	closed   bool
	handlers []RoomBroadcastHandler
}

func NewMemoryRoomBroadcastBus() *MemoryRoomBroadcastBus {
	return &MemoryRoomBroadcastBus{}
}

func (b *MemoryRoomBroadcastBus) PublishRoomEnvelope(ctx context.Context, event RoomBroadcastEvent) error {
	if ctx == nil {
		ctx = context.Background()
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return ErrEventBusDisabled
	}
	handlers := append([]RoomBroadcastHandler(nil), b.handlers...)
	b.mu.Unlock()

	for _, handler := range handlers {
		if handler == nil {
			continue
		}
		handler(ctx, event)
	}
	return nil
}

func (b *MemoryRoomBroadcastBus) SubscribeRoomBroadcasts(ctx context.Context, handler RoomBroadcastHandler) error {
	if handler == nil {
		return nil
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return ErrEventBusDisabled
	}
	b.handlers = append(b.handlers, handler)
	b.mu.Unlock()

	if ctx != nil {
		go func() {
			<-ctx.Done()
		}()
	}
	return nil
}

func (b *MemoryRoomBroadcastBus) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	b.handlers = nil
	return nil
}

type NATSRoomBroadcastBus struct {
	conn    *nats.Conn
	subject string
}

func OpenNATSRoomBroadcastBus(config NATSConfig) (*NATSRoomBroadcastBus, error) {
	config = NormalizeNATSConfig(config)
	conn, err := nats.Connect(
		config.URL,
		nats.Name(config.Name),
		nats.Timeout(2*time.Second),
	)
	if err != nil {
		return nil, err
	}
	return &NATSRoomBroadcastBus{
		conn:    conn,
		subject: config.Subject,
	}, nil
}

func (b *NATSRoomBroadcastBus) PublishRoomEnvelope(ctx context.Context, event RoomBroadcastEvent) error {
	if b == nil || b.conn == nil {
		return ErrEventBusDisabled
	}
	data, err := EncodeRoomBroadcastEvent(event)
	if err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	done := make(chan error, 1)
	go func() {
		done <- b.conn.Publish(b.subject, data)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}

func (b *NATSRoomBroadcastBus) SubscribeRoomBroadcasts(ctx context.Context, handler RoomBroadcastHandler) error {
	if b == nil || b.conn == nil {
		return ErrEventBusDisabled
	}
	if handler == nil {
		return nil
	}
	sub, err := b.conn.Subscribe(b.subject, func(message *nats.Msg) {
		event, err := DecodeRoomBroadcastEvent(message.Data)
		if err != nil {
			return
		}
		handler(context.Background(), event)
	})
	if err != nil {
		return err
	}
	if err := b.conn.Flush(); err != nil {
		_ = sub.Unsubscribe()
		return err
	}
	if ctx != nil {
		go func() {
			<-ctx.Done()
			_ = sub.Unsubscribe()
		}()
	}
	return nil
}

func (b *NATSRoomBroadcastBus) Close() error {
	if b == nil || b.conn == nil {
		return nil
	}
	b.conn.Close()
	return nil
}
