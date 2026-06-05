package eventbus

import (
	"context"
	"encoding/json"
	"testing"
)

func TestNormalizeEventBusDefaultsToNATSCore(t *testing.T) {
	value, err := NormalizeEventBus("")
	if err != nil {
		t.Fatalf("normalize event bus: %v", err)
	}
	if value != EventBusNATSCore {
		t.Fatalf("expected nats_core, got %q", value)
	}
}

func TestNormalizeEventBusRejectsUnsupportedValue(t *testing.T) {
	if _, err := NormalizeEventBus("kafka"); err == nil {
		t.Fatalf("expected unsupported event bus to fail")
	}
}

func TestNormalizeNATSConfigDefaults(t *testing.T) {
	config := NormalizeNATSConfig(NATSConfig{})

	if config.URL != DefaultNATSURL {
		t.Fatalf("expected default url, got %q", config.URL)
	}
	if config.Name != DefaultNATSName {
		t.Fatalf("expected default name, got %q", config.Name)
	}
	if config.Subject != DefaultRoomBroadcastSubject {
		t.Fatalf("expected default subject, got %q", config.Subject)
	}
	if config.ControlSubject != DefaultRoomControlSubject {
		t.Fatalf("expected default control subject, got %q", config.ControlSubject)
	}
}

func TestRoomBroadcastEventRoundTrip(t *testing.T) {
	payload := json.RawMessage(`{"roomId":"ROOM01","seq":4}`)
	event := RoomBroadcastEvent{
		InstanceID:    "roomserver-a",
		RoomID:        "ROOM01",
		Type:          "play",
		Payload:       payload,
		Seq:           4,
		PublishedAtMs: 12345,
	}

	data, err := EncodeRoomBroadcastEvent(event)
	if err != nil {
		t.Fatalf("encode event: %v", err)
	}
	decoded, err := DecodeRoomBroadcastEvent(data)
	if err != nil {
		t.Fatalf("decode event: %v", err)
	}

	if decoded.InstanceID != event.InstanceID ||
		decoded.RoomID != event.RoomID ||
		decoded.Type != event.Type ||
		decoded.Seq != event.Seq ||
		decoded.PublishedAtMs != event.PublishedAtMs ||
		string(decoded.Payload) != string(payload) {
		t.Fatalf("unexpected decoded event: %+v", decoded)
	}
}

func TestDisabledRoomBroadcastBusNoops(t *testing.T) {
	bus := NewDisabledRoomBroadcastBus()

	if err := bus.PublishRoomEnvelope(context.Background(), RoomBroadcastEvent{}); err != nil {
		t.Fatalf("expected disabled publish to no-op, got %v", err)
	}
	if err := bus.SubscribeRoomBroadcasts(context.Background(), func(context.Context, RoomBroadcastEvent) {}); err != nil {
		t.Fatalf("expected disabled subscribe to no-op, got %v", err)
	}
}

func TestMemoryRoomBroadcastBusDeliversEvents(t *testing.T) {
	bus := NewMemoryRoomBroadcastBus()
	defer bus.Close()

	var got RoomBroadcastEvent
	if err := bus.SubscribeRoomBroadcasts(context.Background(), func(_ context.Context, event RoomBroadcastEvent) {
		got = event
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if err := bus.PublishRoomEnvelope(context.Background(), RoomBroadcastEvent{
		InstanceID: "roomserver-b",
		RoomID:     "ROOM01",
		Type:       "room_state",
		Seq:        3,
	}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	if got.RoomID != "ROOM01" || got.Type != "room_state" || got.Seq != 3 {
		t.Fatalf("unexpected delivered event: %+v", got)
	}
}

func TestRoomControlRequestRoundTrip(t *testing.T) {
	payload := json.RawMessage(`{"roomId":"ROOM01","seq":4}`)
	request := RoomControlRequest{
		SourceInstanceID: "roomserver-a",
		RoomID:           "ROOM01",
		UserID:           "user-a",
		DeviceID:         "device-a",
		ConnectionID:     "conn-a",
		Type:             "play",
		Payload:          payload,
		Seq:              4,
		RequestedAtMs:    12345,
	}

	data, err := EncodeRoomControlRequest(request)
	if err != nil {
		t.Fatalf("encode request: %v", err)
	}
	decoded, err := DecodeRoomControlRequest(data)
	if err != nil {
		t.Fatalf("decode request: %v", err)
	}

	if decoded.SourceInstanceID != request.SourceInstanceID ||
		decoded.RoomID != request.RoomID ||
		decoded.UserID != request.UserID ||
		decoded.DeviceID != request.DeviceID ||
		decoded.ConnectionID != request.ConnectionID ||
		decoded.Type != request.Type ||
		decoded.Seq != request.Seq ||
		string(decoded.Payload) != string(payload) {
		t.Fatalf("unexpected decoded request: %+v", decoded)
	}
}

func TestMemoryRoomControlBusRequestReply(t *testing.T) {
	bus := NewMemoryRoomControlBus()
	defer bus.Close()

	if err := bus.SubscribeRoomControls(context.Background(), "roomserver-b", func(_ context.Context, request RoomControlRequest) RoomControlResponse {
		if request.SourceInstanceID != "roomserver-a" || request.RoomID != "ROOM01" {
			return RoomControlResponse{Error: "unexpected request"}
		}
		return RoomControlResponse{
			Type:    request.Type,
			Payload: request.Payload,
			Seq:     request.Seq + 1,
		}
	}); err != nil {
		t.Fatalf("subscribe controls: %v", err)
	}

	response, err := bus.RequestRoomControl(context.Background(), "roomserver-b", RoomControlRequest{
		SourceInstanceID: "roomserver-a",
		RoomID:           "ROOM01",
		Type:             "play",
		Payload:          json.RawMessage(`{"roomId":"ROOM01"}`),
		Seq:              1,
	})
	if err != nil {
		t.Fatalf("request control: %v", err)
	}
	if response.Type != "play" || response.Seq != 2 || response.Error != "" {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestDisabledRoomControlBusRequestFails(t *testing.T) {
	bus := NewDisabledRoomControlBus()

	if _, err := bus.RequestRoomControl(context.Background(), "roomserver-b", RoomControlRequest{}); err == nil {
		t.Fatalf("expected disabled control bus request to fail")
	}
	if err := bus.SubscribeRoomControls(context.Background(), "roomserver-b", func(context.Context, RoomControlRequest) RoomControlResponse {
		return RoomControlResponse{}
	}); err != nil {
		t.Fatalf("expected disabled subscribe to no-op, got %v", err)
	}
}
