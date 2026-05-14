package room

import (
	"testing"
	"time"

	"watch_together/server/internal/realtime"
)

func TestRoomPlayIgnoresClientPositionAsAuthority(t *testing.T) {
	currentTime := time.UnixMilli(1_000)
	room := newTestRoomWithClock("ROOM01", &currentTime)

	state, _, err := room.ApplyPlay("user_a", 50_000)
	if err != nil {
		t.Fatalf("apply play: %v", err)
	}

	if state.PositionMs != 0 {
		t.Fatalf("expected play to derive server position 0, got %d", state.PositionMs)
	}
	if state.Velocity != 1 {
		t.Fatalf("expected play velocity 1, got %f", state.Velocity)
	}
	if state.Paused {
		t.Fatalf("expected playing state after play")
	}
	if state.ServerTimeMs != currentTime.UnixMilli() {
		t.Fatalf("expected serverTimeMs %d, got %d", currentTime.UnixMilli(), state.ServerTimeMs)
	}
	if state.Reason != realtime.ReasonPlay {
		t.Fatalf("expected play reason, got %s", state.Reason)
	}
}

func TestRoomPauseDerivesCurrentPositionBeforeFreezing(t *testing.T) {
	currentTime := time.UnixMilli(1_000)
	room := newTestRoomWithClock("ROOM01", &currentTime)

	if _, _, err := room.ApplyPlay("user_a", 0); err != nil {
		t.Fatalf("apply play: %v", err)
	}

	currentTime = currentTime.Add(4 * time.Second)
	state, _, err := room.ApplyPause("user_a", 99_000)
	if err != nil {
		t.Fatalf("apply pause: %v", err)
	}

	if state.PositionMs != 4_000 {
		t.Fatalf("expected pause to freeze derived position 4000, got %d", state.PositionMs)
	}
	if state.Velocity != 0 {
		t.Fatalf("expected pause velocity 0, got %f", state.Velocity)
	}
	if !state.Paused {
		t.Fatalf("expected paused state after pause")
	}
	if state.Reason != realtime.ReasonPause {
		t.Fatalf("expected pause reason, got %s", state.Reason)
	}
}

func TestRoomSeekUsesTargetAndPreservesVelocity(t *testing.T) {
	currentTime := time.UnixMilli(1_000)
	room := newTestRoomWithClock("ROOM01", &currentTime)

	if _, _, err := room.ApplyPlay("user_a", 0); err != nil {
		t.Fatalf("apply play: %v", err)
	}
	currentTime = currentTime.Add(2 * time.Second)

	state, _, err := room.ApplySeek("user_a", 30_000)
	if err != nil {
		t.Fatalf("apply seek: %v", err)
	}

	if state.PositionMs != 30_000 {
		t.Fatalf("expected seek target position 30000, got %d", state.PositionMs)
	}
	if state.Velocity != 1 {
		t.Fatalf("expected seek to preserve velocity 1, got %f", state.Velocity)
	}
	if state.Paused {
		t.Fatalf("expected seek to keep playing state")
	}
	if state.Reason != realtime.ReasonSeek {
		t.Fatalf("expected seek reason, got %s", state.Reason)
	}
}

func TestRoomRateChangeWhilePausedKeepsVectorPaused(t *testing.T) {
	currentTime := time.UnixMilli(1_000)
	room := newTestRoomWithClock("ROOM01", &currentTime)

	state, _, err := room.ApplyPlaybackRate("user_a", 1.5)
	if err != nil {
		t.Fatalf("apply playback rate: %v", err)
	}

	if state.Velocity != 0 {
		t.Fatalf("expected paused vector velocity 0, got %f", state.Velocity)
	}
	if !state.Paused {
		t.Fatalf("expected paused state to remain paused")
	}
	if state.PlaybackRate != 1.5 {
		t.Fatalf("expected intended playbackRate 1.5, got %f", state.PlaybackRate)
	}
	if state.Reason != realtime.ReasonRateChange {
		t.Fatalf("expected rate_change reason, got %s", state.Reason)
	}
}

func TestRoomEndedUsesRoomSpecificReasonAroundGenericStop(t *testing.T) {
	currentTime := time.UnixMilli(1_000)
	room := newTestRoomWithClock("ROOM01", &currentTime)

	state, _, err := room.ApplyEnded("user_a", 120_000)
	if err != nil {
		t.Fatalf("apply ended: %v", err)
	}

	if state.PositionMs != 120_000 {
		t.Fatalf("expected ended position 120000, got %d", state.PositionMs)
	}
	if state.Velocity != 0 {
		t.Fatalf("expected ended velocity 0, got %f", state.Velocity)
	}
	if !state.Paused || !state.Ended {
		t.Fatalf("expected ended state to be paused and ended")
	}
	if state.Reason != reasonMediaEnd {
		t.Fatalf("expected media_end reason, got %s", state.Reason)
	}
}

func newTestRoomWithClock(id string, currentTime *time.Time) *Room {
	room := newWithClock(id, func() time.Time {
		return *currentTime
	})
	room.state.MediaID = "sample_001"
	room.state.HostUserID = "user_a"
	return room
}
