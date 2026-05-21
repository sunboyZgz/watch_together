package transport

import (
	"testing"
	"time"
)

func TestControlRateLimiterRejectsWithinInterval(t *testing.T) {
	limiter := newControlRateLimiter(250*time.Millisecond, 16, 1)
	now := time.UnixMilli(1_000)

	if !limiter.Reserve("ROOM01", now) {
		t.Fatalf("expected first control to be accepted")
	}
	if limiter.Reserve("ROOM01", now.Add(100*time.Millisecond)) {
		t.Fatalf("expected second control inside interval to be rejected")
	}
	if !limiter.Reserve("ROOM01", now.Add(250*time.Millisecond)) {
		t.Fatalf("expected control at interval boundary to be accepted")
	}
}

func TestControlRateLimiterIsKeyScoped(t *testing.T) {
	limiter := newControlRateLimiter(time.Second, 16, 1)
	now := time.UnixMilli(1_000)

	if !limiter.Reserve("ROOM01", now) {
		t.Fatalf("expected first room control to be accepted")
	}
	if !limiter.Reserve("ROOM02", now.Add(100*time.Millisecond)) {
		t.Fatalf("expected different room to be accepted independently")
	}
}

func TestControlRateLimiterForgetsFailedReservation(t *testing.T) {
	limiter := newControlRateLimiter(time.Second, 16, 1)
	now := time.UnixMilli(1_000)

	if !limiter.Reserve("ROOM01", now) {
		t.Fatalf("expected first control to be accepted")
	}
	limiter.ForgetReservation("ROOM01", now)
	if !limiter.Reserve("ROOM01", now.Add(100*time.Millisecond)) {
		t.Fatalf("expected forgotten reservation to allow retry")
	}
}
