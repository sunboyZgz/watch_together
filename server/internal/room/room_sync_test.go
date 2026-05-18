package room

import (
	"errors"
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

func TestRoomControlExpectedSeqRejectsStaleUpdate(t *testing.T) {
	currentTime := time.UnixMilli(1_000)
	room := newTestRoomWithClock("ROOM01", &currentTime)

	state, _, err := room.ApplyPlayIfSeq("user_a", 0, 1)
	if err != nil {
		t.Fatalf("apply play with expected seq: %v", err)
	}
	if state.Seq != 2 {
		t.Fatalf("expected accepted play to advance seq to 2, got %d", state.Seq)
	}

	state, _, err = room.ApplyPauseIfSeq("user_a", 0, 1)
	if !errors.Is(err, ErrSeqMismatch) {
		t.Fatalf("expected stale pause to return ErrSeqMismatch, got %v", err)
	}
	if state.Seq != 2 {
		t.Fatalf("expected stale pause to return latest seq 2, got %d", state.Seq)
	}
	if state.Paused {
		t.Fatalf("expected stale pause not to overwrite playing state")
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

func TestRoomSeekClampsToMediaDuration(t *testing.T) {
	currentTime := time.UnixMilli(1_000)
	durationMs := int64(120_000)
	room := newTestRoomWithClockAndDuration("ROOM01", &currentTime, &durationMs)

	state, _, err := room.ApplySeek("user_a", 180_000)
	if err != nil {
		t.Fatalf("apply seek: %v", err)
	}

	if state.PositionMs != durationMs {
		t.Fatalf("expected seek to clamp to duration %d, got %d", durationMs, state.PositionMs)
	}
	if !state.Ended {
		t.Fatalf("expected seek beyond duration to expose ended state")
	}
}

func TestRoomPlayAfterEndedAtMediaEndDoesNotAutoReplay(t *testing.T) {
	currentTime := time.UnixMilli(1_000)
	durationMs := int64(120_000)
	room := newTestRoomWithClockAndDuration("ROOM01", &currentTime, &durationMs)

	if _, _, err := room.ApplyEnded("user_a", durationMs); err != nil {
		t.Fatalf("apply ended: %v", err)
	}

	currentTime = currentTime.Add(time.Second)
	state, _, err := room.ApplyPlay("user_a", durationMs)
	if err != nil {
		t.Fatalf("apply play: %v", err)
	}

	if state.PositionMs != durationMs {
		t.Fatalf("expected play at media end to stay at duration %d, got %d", durationMs, state.PositionMs)
	}
	if !state.Ended {
		t.Fatalf("expected play at media end to remain ended")
	}
	if state.Velocity != 0 {
		t.Fatalf("expected play at media end to keep velocity 0, got %f", state.Velocity)
	}
}

func TestRoomReplayRequiresExplicitSeekThenPlay(t *testing.T) {
	currentTime := time.UnixMilli(1_000)
	durationMs := int64(120_000)
	room := newTestRoomWithClockAndDuration("ROOM01", &currentTime, &durationMs)

	if _, _, err := room.ApplyEnded("user_a", durationMs); err != nil {
		t.Fatalf("apply ended: %v", err)
	}
	if _, _, err := room.ApplySeek("user_a", 0); err != nil {
		t.Fatalf("apply seek: %v", err)
	}

	currentTime = currentTime.Add(time.Second)
	state, _, err := room.ApplyPlay("user_a", 0)
	if err != nil {
		t.Fatalf("apply play: %v", err)
	}

	if state.PositionMs != 0 {
		t.Fatalf("expected explicit replay to start at 0, got %d", state.PositionMs)
	}
	if state.Ended {
		t.Fatalf("expected explicit replay to clear ended state")
	}
	if state.Velocity != 1 {
		t.Fatalf("expected explicit replay velocity 1, got %f", state.Velocity)
	}
}

func TestRoomBindMediaDurationWithoutResettingSameMedia(t *testing.T) {
	currentTime := time.UnixMilli(1_000)
	room := newTestRoomWithClock("ROOM01", &currentTime)

	if _, _, err := room.ApplyPlay("user_a", 0); err != nil {
		t.Fatalf("apply play: %v", err)
	}
	currentTime = currentTime.Add(3 * time.Second)

	durationMs := int64(120_000)
	state := room.BindMedia("sample_001", &durationMs)

	if state.PositionMs != 3_000 {
		t.Fatalf("expected media metadata refresh to preserve position 3000, got %d", state.PositionMs)
	}
	if state.MediaDurationMs == nil || *state.MediaDurationMs != durationMs {
		t.Fatalf("expected mediaDurationMs %d, got %v", durationMs, state.MediaDurationMs)
	}
	if state.Seq != 2 {
		t.Fatalf("expected metadata refresh for same media not to change seq, got %d", state.Seq)
	}
}

func TestRoomBindDifferentMediaCreatesNewTimelineVector(t *testing.T) {
	currentTime := time.UnixMilli(1_000)
	room := newTestRoomWithClock("ROOM01", &currentTime)

	if _, _, err := room.ApplyPlay("user_a", 0); err != nil {
		t.Fatalf("apply play: %v", err)
	}
	currentTime = currentTime.Add(3 * time.Second)

	durationMs := int64(240_000)
	state := room.BindMedia("sample_002", &durationMs)

	if state.MediaID != "sample_002" {
		t.Fatalf("expected media sample_002, got %s", state.MediaID)
	}
	if state.PositionMs != 0 {
		t.Fatalf("expected media change to reset position 0, got %d", state.PositionMs)
	}
	if state.Velocity != 0 || !state.Paused {
		t.Fatalf("expected media change to pause timeline, got velocity=%f paused=%t", state.Velocity, state.Paused)
	}
	if state.Seq != 3 {
		t.Fatalf("expected media change seq 3, got %d", state.Seq)
	}
	if state.Reason != reasonMediaChange {
		t.Fatalf("expected media_change reason, got %s", state.Reason)
	}
}

func newTestRoomWithClock(id string, currentTime *time.Time) *Room {
	return newTestRoomWithClockAndDuration(id, currentTime, nil)
}

func newTestRoomWithClockAndDuration(id string, currentTime *time.Time, durationMs *int64) *Room {
	room := newWithClock(id, func() time.Time {
		return *currentTime
	})
	room.state.MediaID = "sample_001"
	room.state.MediaDurationMs = cloneDurationMs(durationMs)
	room.state.HostUserID = "user_a"
	return room
}
