package cache

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestPresenceRegistryDisabled(t *testing.T) {
	registry := NewPresenceRegistry(nil, time.Second)

	_, _, err := registry.Upsert(context.Background(), "ROOM01", "user-a", "host", "device-a", "instance-a", "conn-a", true)
	if !errors.Is(err, ErrRedisDisabled) {
		t.Fatalf("expected ErrRedisDisabled, got %v", err)
	}
}

func TestPresenceRegistryUpsertRefreshAndConflict(t *testing.T) {
	ctx := context.Background()
	registry, redisServer := newTestPresenceRegistry(t)
	defer redisServer.Close()

	member, ok, err := registry.Upsert(ctx, "ROOM01", "user-a", "host", "device-a", "instance-a", "conn-a", true)
	if err != nil {
		t.Fatalf("upsert presence: %v", err)
	}
	if !ok || member.UserID != "user-a" || !member.IsHost || member.ConnectionID != "conn-a" {
		t.Fatalf("unexpected presence member: ok=%t member=%+v", ok, member)
	}
	if !redisServer.Exists(PresenceKey("ROOM01")) {
		t.Fatalf("expected presence key %q to exist", PresenceKey("ROOM01"))
	}

	refreshed, ok, err := registry.Upsert(ctx, "ROOM01", "user-a", "host", "device-a", "instance-b", "conn-b", true)
	if err != nil {
		t.Fatalf("refresh presence: %v", err)
	}
	if !ok || refreshed.InstanceID != "instance-b" || refreshed.ConnectionID != "conn-b" {
		t.Fatalf("expected same device to refresh, ok=%t member=%+v", ok, refreshed)
	}

	conflict, ok, err := registry.Upsert(ctx, "ROOM01", "user-a", "host", "device-b", "instance-c", "conn-c", true)
	if err != nil {
		t.Fatalf("conflict presence: %v", err)
	}
	if ok || conflict.DeviceID != "device-a" || conflict.ConnectionID != "conn-b" {
		t.Fatalf("expected different device conflict, ok=%t member=%+v", ok, conflict)
	}
}

func TestPresenceRegistryReleaseOnlyMatchingDeviceAndConnection(t *testing.T) {
	ctx := context.Background()
	registry, redisServer := newTestPresenceRegistry(t)
	defer redisServer.Close()

	if _, ok, err := registry.Upsert(ctx, "ROOM01", "user-a", "member", "device-a", "instance-a", "conn-a", false); err != nil || !ok {
		t.Fatalf("upsert presence ok=%t err=%v", ok, err)
	}

	released, err := registry.ReleaseIfMatch(ctx, "ROOM01", "user-a", "device-a", "conn-old")
	if err != nil {
		t.Fatalf("release mismatched connection: %v", err)
	}
	if released {
		t.Fatalf("expected mismatched connection release to be ignored")
	}

	released, err = registry.ReleaseIfMatch(ctx, "ROOM01", "user-a", "device-b", "conn-a")
	if err != nil {
		t.Fatalf("release mismatched device: %v", err)
	}
	if released {
		t.Fatalf("expected mismatched device release to be ignored")
	}

	released, err = registry.ReleaseIfMatch(ctx, "ROOM01", "user-a", "device-a", "conn-a")
	if err != nil {
		t.Fatalf("release matching presence: %v", err)
	}
	if !released {
		t.Fatalf("expected matching presence release")
	}
	snapshot, err := registry.Snapshot(ctx, "ROOM01")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snapshot.OnlineCount != 0 {
		t.Fatalf("expected empty snapshot after release, got %+v", snapshot)
	}
}

func TestPresenceRegistrySnapshotDropsExpiredAndSortsHostFirst(t *testing.T) {
	ctx := context.Background()
	now := time.UnixMilli(1_000)
	registry, _ := newTestPresenceRegistryWithClock(t, func() time.Time {
		return now
	})

	if _, ok, err := registry.Upsert(ctx, "ROOM01", "user-b", "member", "device-b", "instance-a", "conn-b", false); err != nil || !ok {
		t.Fatalf("upsert user-b ok=%t err=%v", ok, err)
	}
	if _, ok, err := registry.Upsert(ctx, "ROOM01", "user-a", "host", "device-a", "instance-a", "conn-a", true); err != nil || !ok {
		t.Fatalf("upsert user-a ok=%t err=%v", ok, err)
	}

	snapshot, err := registry.Snapshot(ctx, "ROOM01")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snapshot.OnlineCount != 2 || snapshot.Members[0].UserID != "user-a" || !snapshot.Members[0].IsHost {
		t.Fatalf("expected host-first snapshot, got %+v", snapshot)
	}

	now = now.Add(2 * time.Minute)
	snapshot, err = registry.Snapshot(ctx, "ROOM01")
	if err != nil {
		t.Fatalf("expired snapshot: %v", err)
	}
	if snapshot.OnlineCount != 0 {
		t.Fatalf("expected expired members to be dropped, got %+v", snapshot)
	}
}

func newTestPresenceRegistry(t *testing.T) (*PresenceRegistry, *miniredis.Miniredis) {
	return newTestPresenceRegistryWithClock(t, time.Now)
}

func newTestPresenceRegistryWithClock(
	t *testing.T,
	now func() time.Time,
) (*PresenceRegistry, *miniredis.Miniredis) {
	t.Helper()
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})
	return NewPresenceRegistryWithClock(&RedisClient{client: client}, time.Minute, now), redisServer
}
