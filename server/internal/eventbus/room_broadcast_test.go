package eventbus

import (
	"context"
	"encoding/json"
	"testing"
)

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
