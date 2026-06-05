package cache

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestActiveDeviceRegistryDisabled(t *testing.T) {
	registry := NewActiveDeviceRegistry(nil, time.Second)

	_, _, err := registry.Acquire(context.Background(), "ROOM01", "user-a", "device-a", "instance-a", "conn-a")
	if !errors.Is(err, ErrRedisDisabled) {
		t.Fatalf("expected ErrRedisDisabled, got %v", err)
	}
}

func TestActiveDeviceRegistryAcquireRefreshAndConflict(t *testing.T) {
	ctx := context.Background()
	registry, redisServer := newTestActiveDeviceRegistry(t)
	defer redisServer.Close()

	lease, acquired, err := registry.Acquire(ctx, "ROOM01", "user-a", "device-a", "instance-a", "conn-a")
	if err != nil {
		t.Fatalf("acquire active device: %v", err)
	}
	if !acquired || lease.DeviceID != "device-a" || lease.ConnectionID != "conn-a" {
		t.Fatalf("unexpected acquired lease: acquired=%t lease=%+v", acquired, lease)
	}
	if !redisServer.Exists(ActiveDeviceKey("ROOM01", "user-a")) {
		t.Fatalf("expected active-device key %q to exist", ActiveDeviceKey("ROOM01", "user-a"))
	}

	refreshed, refreshedOK, err := registry.Acquire(ctx, "ROOM01", "user-a", "device-a", "instance-b", "conn-b")
	if err != nil {
		t.Fatalf("refresh same device: %v", err)
	}
	if !refreshedOK || refreshed.DeviceID != "device-a" || refreshed.ConnectionID != "conn-b" || refreshed.InstanceID != "instance-b" {
		t.Fatalf("expected same device to refresh ownership, got ok=%t lease=%+v", refreshedOK, refreshed)
	}

	conflict, acquiredByOther, err := registry.Acquire(ctx, "ROOM01", "user-a", "device-b", "instance-c", "conn-c")
	if err != nil {
		t.Fatalf("acquire different device: %v", err)
	}
	if acquiredByOther {
		t.Fatalf("expected different device to conflict")
	}
	if conflict.DeviceID != "device-a" || conflict.ConnectionID != "conn-b" {
		t.Fatalf("expected existing lease in conflict response, got %+v", conflict)
	}
}

func TestActiveDeviceRegistryReleaseOnlyMatchingDeviceAndConnection(t *testing.T) {
	ctx := context.Background()
	registry, redisServer := newTestActiveDeviceRegistry(t)
	defer redisServer.Close()

	if _, acquired, err := registry.Acquire(ctx, "ROOM01", "user-a", "device-a", "instance-a", "conn-a"); err != nil || !acquired {
		t.Fatalf("acquire active device acquired=%t err=%v", acquired, err)
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
	if !redisServer.Exists(ActiveDeviceKey("ROOM01", "user-a")) {
		t.Fatalf("expected active-device lease to remain after mismatched release")
	}

	released, err = registry.ReleaseIfMatch(ctx, "ROOM01", "user-a", "device-a", "conn-a")
	if err != nil {
		t.Fatalf("release matching active device: %v", err)
	}
	if !released || redisServer.Exists(ActiveDeviceKey("ROOM01", "user-a")) {
		t.Fatalf("expected matching release to delete active-device lease")
	}
}

func newTestActiveDeviceRegistry(t *testing.T) (*ActiveDeviceRegistry, *miniredis.Miniredis) {
	t.Helper()
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})
	return NewActiveDeviceRegistry(&RedisClient{client: client}, time.Minute), redisServer
}
