package transport

import (
	"testing"
	"time"
)

func TestControlRequestDeduperDeduplicatesWithinTTL(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	deduper := newControlRequestDeduper(time.Minute, 16, 1)

	if !deduper.Reserve("room_1", "req_1", now) {
		t.Fatalf("expected first request to be accepted")
	}
	if deduper.Reserve("room_1", "req_1", now.Add(time.Second)) {
		t.Fatalf("expected duplicate request to be deduplicated")
	}
	if !deduper.Reserve("room_1", "req_1", now.Add(2*time.Minute)) {
		t.Fatalf("expected expired request id to be accepted again")
	}
}

func TestControlRequestDeduperForgetAllowsRetry(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	deduper := newControlRequestDeduper(time.Minute, 16, 1)

	if !deduper.Reserve("room_1", "req_1", now) {
		t.Fatalf("expected first request to be accepted")
	}
	deduper.Forget("room_1", "req_1")
	if !deduper.Reserve("room_1", "req_1", now.Add(time.Second)) {
		t.Fatalf("expected forgotten request id to be accepted again")
	}
}

func TestControlRequestDeduperFailsOpenWhenShardIsFull(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	deduper := newControlRequestDeduper(time.Minute, 1, 1)

	if !deduper.Reserve("room_1", "req_1", now) {
		t.Fatalf("expected first request to be accepted")
	}
	if !deduper.Reserve("room_1", "req_2", now.Add(time.Second)) {
		t.Fatalf("expected saturated deduper to fail open")
	}
	if !deduper.Reserve("room_1", "req_2", now.Add(2*time.Second)) {
		t.Fatalf("expected fail-open request not to be remembered")
	}
}

func TestControlRequestDeduperCleansExpiredEntriesAtCapacity(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	deduper := newControlRequestDeduper(time.Second, 1, 1)

	if !deduper.Reserve("room_1", "req_1", now) {
		t.Fatalf("expected first request to be accepted")
	}
	if !deduper.Reserve("room_1", "req_2", now.Add(2*time.Second)) {
		t.Fatalf("expected expired entry to be cleaned at capacity")
	}
	if deduper.Reserve("room_1", "req_2", now.Add(2500*time.Millisecond)) {
		t.Fatalf("expected newly stored request id to deduplicate")
	}
}
