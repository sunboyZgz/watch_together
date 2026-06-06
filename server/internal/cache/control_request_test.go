package cache

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestControlRequestRegistryDisabled(t *testing.T) {
	registry := NewControlRequestRegistry(nil, time.Second)

	_, _, err := registry.Reserve(context.Background(), "ROOM01", "req-1", 1)
	if !errors.Is(err, ErrRedisDisabled) {
		t.Fatalf("expected ErrRedisDisabled, got %v", err)
	}
}

func TestControlRequestRegistryReserveAndDuplicatePending(t *testing.T) {
	ctx := context.Background()
	registry, redisServer := newTestControlRequestRegistry(t)
	defer redisServer.Close()

	record, reserved, err := registry.Reserve(ctx, "ROOM01", "req-1", 3)
	if err != nil {
		t.Fatalf("reserve request: %v", err)
	}
	if !reserved || record.Status != ControlRequestStatusPending || record.AuthorityEpoch != 3 {
		t.Fatalf("unexpected reserved record: reserved=%t record=%+v", reserved, record)
	}
	if !redisServer.Exists(ControlRequestKey("ROOM01", "req-1")) {
		t.Fatalf("expected control request key to exist")
	}

	duplicate, reserved, err := registry.Reserve(ctx, "ROOM01", "req-1", 3)
	if err != nil {
		t.Fatalf("reserve duplicate: %v", err)
	}
	if reserved || duplicate.Status != ControlRequestStatusPending {
		t.Fatalf("expected duplicate pending record, reserved=%t record=%+v", reserved, duplicate)
	}
}

func TestControlRequestRegistryFinalizeAcceptedAndDuplicate(t *testing.T) {
	ctx := context.Background()
	registry, _ := newTestControlRequestRegistry(t)

	if _, reserved, err := registry.Reserve(ctx, "ROOM01", "req-1", 4); err != nil || !reserved {
		t.Fatalf("reserve request reserved=%t err=%v", reserved, err)
	}

	envelope := []byte(`{"type":"play","payload":{"roomId":"ROOM01","seq":2}}`)
	accepted, finalized, err := registry.FinalizeAccepted(ctx, "ROOM01", "req-1", 4, 2, envelope)
	if err != nil {
		t.Fatalf("finalize accepted: %v", err)
	}
	if !finalized || accepted.Status != ControlRequestStatusAccepted || accepted.Seq != 2 {
		t.Fatalf("unexpected accepted record: finalized=%t record=%+v", finalized, accepted)
	}
	var decoded map[string]any
	if err := json.Unmarshal(accepted.Envelope, &decoded); err != nil {
		t.Fatalf("expected stored envelope json: %v", err)
	}

	duplicate, reserved, err := registry.Reserve(ctx, "ROOM01", "req-1", 4)
	if err != nil {
		t.Fatalf("reserve duplicate accepted: %v", err)
	}
	if reserved || duplicate.Status != ControlRequestStatusAccepted || duplicate.Seq != 2 {
		t.Fatalf("expected duplicate accepted record, reserved=%t record=%+v", reserved, duplicate)
	}
}

func TestControlRequestRegistryFinalizeRejectedAndStaleEpoch(t *testing.T) {
	ctx := context.Background()
	registry, _ := newTestControlRequestRegistry(t)

	if _, reserved, err := registry.Reserve(ctx, "ROOM01", "req-1", 5); err != nil || !reserved {
		t.Fatalf("reserve request reserved=%t err=%v", reserved, err)
	}

	stale, finalized, err := registry.FinalizeAccepted(ctx, "ROOM01", "req-1", 4, 2, []byte(`{}`))
	if err != nil {
		t.Fatalf("stale finalize: %v", err)
	}
	if finalized || stale.Status != ControlRequestStatusPending {
		t.Fatalf("expected stale epoch finalize to be ignored, finalized=%t record=%+v", finalized, stale)
	}

	rejected, finalized, err := registry.FinalizeRejected(ctx, "ROOM01", "req-1", 5, 1, "room authority unavailable")
	if err != nil {
		t.Fatalf("finalize rejected: %v", err)
	}
	if !finalized || rejected.Status != ControlRequestStatusRejected || rejected.Error != "room authority unavailable" {
		t.Fatalf("unexpected rejected record: finalized=%t record=%+v", finalized, rejected)
	}
}

func newTestControlRequestRegistry(t *testing.T) (*ControlRequestRegistry, *miniredis.Miniredis) {
	t.Helper()
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})
	return NewControlRequestRegistry(&RedisClient{client: client}, time.Minute), redisServer
}
