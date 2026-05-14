package transport

import (
	"testing"

	"watch_together/server/internal/room"
)

func TestRoomStatePayloadDerivesPausedFromVelocity(t *testing.T) {
	state := room.State{
		RoomID:       "ROOM01",
		MediaID:      "sample_001",
		HostUserID:   "user_a",
		Paused:       true,
		PositionMs:   10_000,
		Velocity:     1,
		ServerTimeMs: 1_000,
		PlaybackRate: 1,
		Seq:          2,
		Reason:       "play",
	}

	payload := roomStatePayload(state)

	if payload.Paused {
		t.Fatalf("expected paused=false when velocity is non-zero")
	}
	if payload.Velocity != 1 {
		t.Fatalf("expected velocity 1, got %f", payload.Velocity)
	}
}

func TestRoomStateResponseDerivesPausedFromVelocity(t *testing.T) {
	state := room.State{
		Paused:       false,
		PositionMs:   10_000,
		Velocity:     0,
		ServerTimeMs: 1_000,
		PlaybackRate: 1,
		Seq:          2,
		Reason:       "pause",
	}

	response := roomStateToResponse(state)

	if !response.Paused {
		t.Fatalf("expected paused=true when velocity is zero")
	}
	if response.Velocity != 0 {
		t.Fatalf("expected velocity 0, got %f", response.Velocity)
	}
}
