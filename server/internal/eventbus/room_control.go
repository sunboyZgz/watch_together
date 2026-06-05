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

type RoomControlRequest struct {
	SourceInstanceID string          `json:"sourceInstanceId"`
	RoomID           string          `json:"roomId"`
	UserID           string          `json:"userId"`
	DeviceID         string          `json:"deviceId"`
	ConnectionID     string          `json:"connectionId"`
	Type             string          `json:"type"`
	Payload          json.RawMessage `json:"payload"`
	RequestID        string          `json:"requestId,omitempty"`
	Seq              int64           `json:"seq,omitempty"`
	AuthorityEpoch   int64           `json:"authorityEpoch,omitempty"`
	RequestedAtMs    int64           `json:"requestedAtMs"`
}

type RoomControlResponse struct {
	Type           string          `json:"type,omitempty"`
	Payload        json.RawMessage `json:"payload,omitempty"`
	Seq            int64           `json:"seq,omitempty"`
	AuthorityEpoch int64           `json:"authorityEpoch,omitempty"`
	Error          string          `json:"error,omitempty"`
}

type RoomControlHandler func(ctx context.Context, request RoomControlRequest) RoomControlResponse

type RoomControlBus interface {
	RequestRoomControl(ctx context.Context, authorityInstanceID string, request RoomControlRequest) (RoomControlResponse, error)
	SubscribeRoomControls(ctx context.Context, instanceID string, handler RoomControlHandler) error
	Close() error
}

func EncodeRoomControlRequest(request RoomControlRequest) ([]byte, error) {
	return json.Marshal(request)
}

func DecodeRoomControlRequest(data []byte) (RoomControlRequest, error) {
	var request RoomControlRequest
	if err := json.Unmarshal(data, &request); err != nil {
		return RoomControlRequest{}, err
	}
	return request, nil
}

func EncodeRoomControlResponse(response RoomControlResponse) ([]byte, error) {
	return json.Marshal(response)
}

func DecodeRoomControlResponse(data []byte) (RoomControlResponse, error) {
	var response RoomControlResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return RoomControlResponse{}, err
	}
	return response, nil
}

type DisabledRoomControlBus struct{}

func NewDisabledRoomControlBus() DisabledRoomControlBus {
	return DisabledRoomControlBus{}
}

func (DisabledRoomControlBus) RequestRoomControl(
	context.Context,
	string,
	RoomControlRequest,
) (RoomControlResponse, error) {
	return RoomControlResponse{}, ErrEventBusDisabled
}

func (DisabledRoomControlBus) SubscribeRoomControls(context.Context, string, RoomControlHandler) error {
	return nil
}

func (DisabledRoomControlBus) Close() error {
	return nil
}

type MemoryRoomControlBus struct {
	mu       sync.Mutex
	closed   bool
	handlers map[string]RoomControlHandler
}

func NewMemoryRoomControlBus() *MemoryRoomControlBus {
	return &MemoryRoomControlBus{
		handlers: make(map[string]RoomControlHandler),
	}
}

func (b *MemoryRoomControlBus) RequestRoomControl(
	ctx context.Context,
	authorityInstanceID string,
	request RoomControlRequest,
) (RoomControlResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return RoomControlResponse{}, ErrEventBusDisabled
	}
	handler := b.handlers[authorityInstanceID]
	b.mu.Unlock()
	if handler == nil {
		return RoomControlResponse{}, ErrEventBusDisabled
	}

	responseCh := make(chan RoomControlResponse, 1)
	go func() {
		responseCh <- handler(ctx, request)
	}()
	select {
	case <-ctx.Done():
		return RoomControlResponse{}, ctx.Err()
	case response := <-responseCh:
		return response, nil
	}
}

func (b *MemoryRoomControlBus) SubscribeRoomControls(
	ctx context.Context,
	instanceID string,
	handler RoomControlHandler,
) error {
	if strings.TrimSpace(instanceID) == "" || handler == nil {
		return nil
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return ErrEventBusDisabled
	}
	b.handlers[instanceID] = handler
	b.mu.Unlock()

	if ctx != nil {
		go func() {
			<-ctx.Done()
			b.mu.Lock()
			delete(b.handlers, instanceID)
			b.mu.Unlock()
		}()
	}
	return nil
}

func (b *MemoryRoomControlBus) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	b.handlers = nil
	return nil
}

type NATSRoomControlBus struct {
	conn         *nats.Conn
	controlTopic string
	timeout      time.Duration
}

func OpenNATSRoomControlBus(config NATSConfig) (*NATSRoomControlBus, error) {
	config = NormalizeNATSConfig(config)
	conn, err := nats.Connect(
		config.URL,
		nats.Name(config.Name+"-control"),
		nats.Timeout(2*time.Second),
	)
	if err != nil {
		return nil, err
	}
	return &NATSRoomControlBus{
		conn:         conn,
		controlTopic: config.ControlSubject,
		timeout:      3 * time.Second,
	}, nil
}

func (b *NATSRoomControlBus) RequestRoomControl(
	ctx context.Context,
	authorityInstanceID string,
	request RoomControlRequest,
) (RoomControlResponse, error) {
	if b == nil || b.conn == nil {
		return RoomControlResponse{}, ErrEventBusDisabled
	}
	if strings.TrimSpace(authorityInstanceID) == "" {
		return RoomControlResponse{}, errors.New("authority instance id is required")
	}
	data, err := EncodeRoomControlRequest(request)
	if err != nil {
		return RoomControlResponse{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := b.timeout
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining > 0 && remaining < timeout {
			timeout = remaining
		}
	}

	responseCh := make(chan struct {
		message *nats.Msg
		err     error
	}, 1)
	go func() {
		message, err := b.conn.Request(roomControlSubject(b.controlTopic, authorityInstanceID), data, timeout)
		responseCh <- struct {
			message *nats.Msg
			err     error
		}{message: message, err: err}
	}()
	select {
	case <-ctx.Done():
		return RoomControlResponse{}, ctx.Err()
	case result := <-responseCh:
		if result.err != nil {
			return RoomControlResponse{}, result.err
		}
		return DecodeRoomControlResponse(result.message.Data)
	}
}

func (b *NATSRoomControlBus) SubscribeRoomControls(
	ctx context.Context,
	instanceID string,
	handler RoomControlHandler,
) error {
	if b == nil || b.conn == nil {
		return ErrEventBusDisabled
	}
	if strings.TrimSpace(instanceID) == "" || handler == nil {
		return nil
	}
	sub, err := b.conn.Subscribe(roomControlSubject(b.controlTopic, instanceID), func(message *nats.Msg) {
		request, err := DecodeRoomControlRequest(message.Data)
		if err != nil {
			return
		}
		response := handler(context.Background(), request)
		data, err := EncodeRoomControlResponse(response)
		if err != nil {
			return
		}
		_ = message.Respond(data)
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

func (b *NATSRoomControlBus) Close() error {
	if b == nil || b.conn == nil {
		return nil
	}
	b.conn.Close()
	return nil
}

func roomControlSubject(base string, instanceID string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = DefaultRoomControlSubject
	}
	instanceID = strings.TrimSpace(instanceID)
	instanceID = strings.ReplaceAll(instanceID, " ", "_")
	return base + "." + instanceID
}
