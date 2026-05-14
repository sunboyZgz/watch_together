package realtime

import "time"

type Clock interface {
	Now() time.Time
	NowUnixMilli() int64
}

type SystemClock struct{}

func (SystemClock) Now() time.Time {
	return time.Now()
}

func (c SystemClock) NowUnixMilli() int64 {
	return c.Now().UnixMilli()
}

type ClockFunc func() time.Time

func (f ClockFunc) Now() time.Time {
	return f()
}

func (f ClockFunc) NowUnixMilli() int64 {
	return f().UnixMilli()
}
