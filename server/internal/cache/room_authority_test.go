package cache

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestRoomAuthorityRegistryDisabled(t *testing.T) {
	registry := NewRoomAuthorityRegistry(nil, time.Second)

	if _, _, err := registry.ClaimAuthority(context.Background(), "ROOM01", "instance-a"); !errors.Is(err, ErrRedisDisabled) {
		t.Fatalf("expected ErrRedisDisabled, got %v", err)
	}
}

func TestRoomAuthorityRegistryClaimRefreshAndRejectsOtherInstance(t *testing.T) {
	ctx := context.Background()
	registry, redisServer := newTestRoomAuthorityRegistry(t)
	defer redisServer.Close()

	lease, claimed, err := registry.ClaimAuthority(ctx, "ROOM01", "instance-a")
	if err != nil {
		t.Fatalf("claim authority: %v", err)
	}
	if !claimed || lease.InstanceID != "instance-a" {
		t.Fatalf("expected instance-a to claim authority, got claimed=%t lease=%+v", claimed, lease)
	}

	if !redisServer.Exists(RoomAuthorityKey("ROOM01")) {
		t.Fatalf("expected authority key %q to exist", RoomAuthorityKey("ROOM01"))
	}

	refreshed, refreshedOK, err := registry.RefreshAuthority(ctx, "ROOM01", "instance-a")
	if err != nil {
		t.Fatalf("refresh authority: %v", err)
	}
	if !refreshedOK || refreshed.InstanceID != "instance-a" {
		t.Fatalf("expected same instance refresh, got ok=%t lease=%+v", refreshedOK, refreshed)
	}

	current, claimedByOther, err := registry.ClaimAuthority(ctx, "ROOM01", "instance-b")
	if err != nil {
		t.Fatalf("claim authority from other instance: %v", err)
	}
	if claimedByOther {
		t.Fatalf("expected duplicate authority claim to be rejected")
	}
	if current.InstanceID != "instance-a" {
		t.Fatalf("expected existing authority to remain instance-a, got %+v", current)
	}
}

func TestRoomAuthorityRegistryReleaseOnlyMatchingInstance(t *testing.T) {
	ctx := context.Background()
	registry, redisServer := newTestRoomAuthorityRegistry(t)
	defer redisServer.Close()

	if _, claimed, err := registry.ClaimAuthority(ctx, "ROOM01", "instance-a"); err != nil || !claimed {
		t.Fatalf("claim authority claimed=%t err=%v", claimed, err)
	}
	released, err := registry.ReleaseAuthority(ctx, "ROOM01", "instance-b")
	if err != nil {
		t.Fatalf("release authority from other instance: %v", err)
	}
	if released {
		t.Fatalf("expected release from other instance to be ignored")
	}
	if !redisServer.Exists(RoomAuthorityKey("ROOM01")) {
		t.Fatalf("expected authority lease to remain after mismatched release")
	}

	released, err = registry.ReleaseAuthority(ctx, "ROOM01", "instance-a")
	if err != nil {
		t.Fatalf("release authority: %v", err)
	}
	if !released || redisServer.Exists(RoomAuthorityKey("ROOM01")) {
		t.Fatalf("expected matching release to delete authority lease")
	}
}

func newTestRoomAuthorityRegistry(t *testing.T) (*RoomAuthorityRegistry, *miniredis.Miniredis) {
	t.Helper()
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})
	return NewRoomAuthorityRegistry(&RedisClient{client: client}, time.Minute), redisServer
}
