package realtime

import (
	"math"
	"time"
)

const (
	ReasonInit       = "init"
	ReasonPlay       = "play"
	ReasonPause      = "pause"
	ReasonSeek       = "seek"
	ReasonRateChange = "rate_change"
	ReasonEnd        = "end"
)

type TimelineBounds struct {
	StartMs int64
	EndMs   *int64
}

type TimelineVector struct {
	PositionMs   int64
	Velocity     float64
	ServerTimeMs int64
	Seq          int64
	Reason       string
	Bounds       *TimelineBounds
}

func NewTimelineVector(now time.Time) TimelineVector {
	return NewTimelineVectorWithBounds(now, nil)
}

func NewTimelineVectorWithBounds(now time.Time, bounds *TimelineBounds) TimelineVector {
	return TimelineVector{
		PositionMs:   0,
		Velocity:     0,
		ServerTimeMs: now.UnixMilli(),
		Seq:          1,
		Reason:       ReasonInit,
		Bounds:       bounds,
	}
}

func (v TimelineVector) CurrentPositionMs(now time.Time) int64 {
	position := v.PositionMs
	if v.Velocity != 0 {
		elapsedMs := now.UnixMilli() - v.ServerTimeMs
		if elapsedMs > 0 {
			position += int64(math.Round(float64(elapsedMs) * v.Velocity))
		}
	}
	return ClampPosition(position, v.Bounds)
}

func (v TimelineVector) SnapshotAt(now time.Time) TimelineVector {
	v.PositionMs = v.CurrentPositionMs(now)
	v.ServerTimeMs = now.UnixMilli()
	return v
}

func Play(previous TimelineVector, now time.Time, velocity float64) TimelineVector {
	if velocity <= 0 || math.IsNaN(velocity) || math.IsInf(velocity, 0) {
		velocity = 1
	}
	next := previous.SnapshotAt(now)
	next.Velocity = velocity
	next.Seq = previous.Seq + 1
	next.Reason = ReasonPlay
	return next
}

func Pause(previous TimelineVector, now time.Time) TimelineVector {
	next := previous.SnapshotAt(now)
	next.Velocity = 0
	next.Seq = previous.Seq + 1
	next.Reason = ReasonPause
	return next
}

func Seek(previous TimelineVector, now time.Time, positionMs int64) TimelineVector {
	next := previous.SnapshotAt(now)
	next.PositionMs = ClampPosition(positionMs, previous.Bounds)
	next.Seq = previous.Seq + 1
	next.Reason = ReasonSeek
	return next
}

func RateChange(previous TimelineVector, now time.Time, velocity float64) TimelineVector {
	next := previous.SnapshotAt(now)
	next.Velocity = velocity
	next.Seq = previous.Seq + 1
	next.Reason = ReasonRateChange
	return next
}

func End(previous TimelineVector, now time.Time, positionMs int64) TimelineVector {
	return StopAt(previous, now, positionMs, ReasonEnd)
}

func StopAt(previous TimelineVector, now time.Time, positionMs int64, reason string) TimelineVector {
	currentPosition := previous.CurrentPositionMs(now)
	if currentPosition > positionMs {
		positionMs = currentPosition
	}
	next := previous.SnapshotAt(now)
	next.PositionMs = ClampPosition(positionMs, previous.Bounds)
	next.Velocity = 0
	next.Seq = previous.Seq + 1
	if reason == "" {
		reason = ReasonEnd
	}
	next.Reason = reason
	return next
}

func ClampPosition(positionMs int64, bounds *TimelineBounds) int64 {
	if bounds == nil {
		if positionMs < 0 {
			return 0
		}
		return positionMs
	}
	if positionMs < bounds.StartMs {
		return bounds.StartMs
	}
	if bounds.EndMs != nil && *bounds.EndMs >= bounds.StartMs && positionMs > *bounds.EndMs {
		return *bounds.EndMs
	}
	return positionMs
}
