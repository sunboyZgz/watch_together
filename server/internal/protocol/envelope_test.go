package protocol

import "testing"

func TestEnvelopeOutboxCoalesceKey(t *testing.T) {
	if key := (Envelope{Type: TypeRoomState}).OutboxCoalesceKey(); key != TypeRoomState {
		t.Fatalf("expected room_state coalesce key, got %q", key)
	}
	if key := (Envelope{Type: TypePlay}).OutboxCoalesceKey(); key != "" {
		t.Fatalf("expected play to be non-coalescable, got %q", key)
	}
}
