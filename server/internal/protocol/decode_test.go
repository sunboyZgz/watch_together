package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDecodeClockSyncPing(t *testing.T) {
	envelope := Envelope{
		Type: TypeClockSyncPing,
		Payload: mustRawMessage(t, ClockSyncPingPayload{
			ClientSendMonoMs: 123_456,
		}),
	}

	payload, err := DecodeClockSyncPing(envelope)
	if err != nil {
		t.Fatalf("decode clock sync ping: %v", err)
	}
	if payload.ClientSendMonoMs != 123_456 {
		t.Fatalf("expected clientSendMonoMs 123456, got %d", payload.ClientSendMonoMs)
	}
}

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

func mustRawMessage(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return data
}
