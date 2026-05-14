package realtime

import (
	"testing"
	"time"
)

func TestTimelineCurrentPositionDerivesFromVelocityAndServerTime(t *testing.T) {
	state := TimelineVector{
		PositionMs:   100_000,
		Velocity:     1.25,
		ServerTimeMs: 10_000,
		Seq:          7,
	}

	position := state.CurrentPositionMs(time.UnixMilli(14_000))

	if position != 105_000 {
		t.Fatalf("expected derived position 105000, got %d", position)
	}
}

func TestPauseDerivesCurrentPositionBeforeFreezing(t *testing.T) {
	previous := TimelineVector{
		PositionMs:   100_000,
		Velocity:     1,
		ServerTimeMs: 10_000,
		Seq:          7,
	}

	next := Pause(previous, time.UnixMilli(15_000))

	if next.PositionMs != 105_000 {
		t.Fatalf("expected paused position 105000, got %d", next.PositionMs)
	}
	if next.Velocity != 0 {
		t.Fatalf("expected velocity 0, got %f", next.Velocity)
	}
	if next.ServerTimeMs != 15_000 {
		t.Fatalf("expected server time 15000, got %d", next.ServerTimeMs)
	}
	if next.Seq != 8 {
		t.Fatalf("expected seq 8, got %d", next.Seq)
	}
	if next.Reason != ReasonPause {
		t.Fatalf("expected reason pause, got %s", next.Reason)
	}
}

func TestSeekPreservesVelocity(t *testing.T) {
	previous := TimelineVector{
		PositionMs:   100_000,
		Velocity:     1,
		ServerTimeMs: 10_000,
		Seq:          7,
	}

	next := Seek(previous, time.UnixMilli(15_000), 30_000)

	if next.PositionMs != 30_000 {
		t.Fatalf("expected seek position 30000, got %d", next.PositionMs)
	}
	if next.Velocity != 1 {
		t.Fatalf("expected seek to preserve velocity 1, got %f", next.Velocity)
	}
	if next.Seq != 8 {
		t.Fatalf("expected seq 8, got %d", next.Seq)
	}
}

func TestTimelineBoundsClampDerivedPosition(t *testing.T) {
	endMs := int64(104_000)
	state := TimelineVector{
		PositionMs:   100_000,
		Velocity:     1.25,
		ServerTimeMs: 10_000,
		Bounds: &TimelineBounds{
			StartMs: 10_000,
			EndMs:   &endMs,
		},
	}

	position := state.CurrentPositionMs(time.UnixMilli(14_000))

	if position != 104_000 {
		t.Fatalf("expected clamped position 104000, got %d", position)
	}
}

func TestStopAtUsesGenericReason(t *testing.T) {
	previous := TimelineVector{
		PositionMs:   100_000,
		Velocity:     1,
		ServerTimeMs: 10_000,
		Seq:          7,
	}

	next := StopAt(previous, time.UnixMilli(15_000), 110_000, "policy_stop")

	if next.PositionMs != 110_000 {
		t.Fatalf("expected stopped position 110000, got %d", next.PositionMs)
	}
	if next.Velocity != 0 {
		t.Fatalf("expected stopped velocity 0, got %f", next.Velocity)
	}
	if next.Reason != "policy_stop" {
		t.Fatalf("expected reason policy_stop, got %s", next.Reason)
	}
}
