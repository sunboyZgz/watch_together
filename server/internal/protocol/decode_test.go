package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDecodeClockSyncPingRejectsMissingClientSendMonoMs(t *testing.T) {
	envelope := Envelope{
		Type:    TypeClockSyncPing,
		Payload: mustRawMessage(t, ClockSyncPingPayload{}),
	}

	_, err := DecodeClockSyncPing(envelope)
	if err == nil || !strings.Contains(err.Error(), "missing clientSendMonoMs") {
		t.Fatalf("expected missing clientSendMonoMs error, got %v", err)
	}
}

func TestDecodeClockSyncPingRejectsWrongType(t *testing.T) {
	envelope := Envelope{
		Type: TypeHeartbeatAck,
		Payload: mustRawMessage(t, ClockSyncPingPayload{
			ClientSendMonoMs: 123_456,
		}),
	}

	_, err := DecodeClockSyncPing(envelope)
	if err == nil || !strings.Contains(err.Error(), TypeHeartbeatAck) {
		t.Fatalf("expected unsupported type error, got %v", err)
	}
}

func TestDecodeJoinRoomRejectsMissingDeviceID(t *testing.T) {
	envelope := Envelope{
		Type: TypeJoinRoom,
		Payload: mustRawMessage(t, JoinRoomPayload{
			RoomID: "room_001",
			UserID: "user_a",
		}),
	}

	_, err := DecodeJoinRoom(envelope)
	if err == nil || !strings.Contains(err.Error(), "missing deviceId") {
		t.Fatalf("expected missing deviceId error, got %v", err)
	}
}

func TestDecodePlayKeepsOptionalRequestID(t *testing.T) {
	envelope := Envelope{
		Type: TypePlay,
		Payload: mustRawMessage(t, PlayPayload{
			RoomID:     "room_001",
			UserID:     "user_a",
			RequestID:  "req-123",
			PositionMs: 1_000,
			Seq:        3,
		}),
	}

	payload, err := DecodePlay(envelope)
	if err != nil {
		t.Fatalf("decode play: %v", err)
	}
	if payload.RequestID != "req-123" {
		t.Fatalf("expected requestId req-123, got %q", payload.RequestID)
	}
}

func mustRawMessage(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return data
}
