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
	if lease.Epoch != 1 || lease.Status != RoomAuthorityStatusActive {
		t.Fatalf("expected initial active epoch 1, got %+v", lease)
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
	if refreshed.Epoch != lease.Epoch || refreshed.Status != RoomAuthorityStatusActive {
		t.Fatalf("expected refresh to keep active epoch, got %+v after %+v", refreshed, lease)
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

func TestRoomAuthorityRegistryRecoveryEpochLifecycle(t *testing.T) {
	ctx := context.Background()
	now := time.UnixMilli(1_000)
	registry, redisServer := newTestRoomAuthorityRegistryWithClock(t, func() time.Time {
		return now
	})
	defer redisServer.Close()

	claimed, ok, err := registry.ClaimAuthority(ctx, "ROOM01", "instance-a")
	if err != nil || !ok {
		t.Fatalf("claim authority ok=%t err=%v", ok, err)
	}

	recovery, started, err := registry.BeginRecovery(ctx, "ROOM01", "instance-b")
	if err != nil {
		t.Fatalf("begin recovery before expiry: %v", err)
	}
	if started || recovery.InstanceID != "instance-a" {
		t.Fatalf("expected healthy authority to reject takeover, started=%t lease=%+v", started, recovery)
	}

	now = now.Add(2 * time.Minute)
	recovery, started, err = registry.BeginRecovery(ctx, "ROOM01", "instance-b")
	if err != nil {
		t.Fatalf("begin recovery after expiry: %v", err)
	}
	if !started || recovery.InstanceID != "instance-b" ||
		recovery.Status != RoomAuthorityStatusRecovering ||
		recovery.Epoch != claimed.Epoch+1 {
		t.Fatalf("unexpected recovery lease: started=%t lease=%+v previous=%+v", started, recovery, claimed)
	}

	renewed, renewedOK, err := registry.RenewAuthorityEpoch(ctx, "ROOM01", "instance-a", claimed.Epoch)
	if err != nil {
		t.Fatalf("stale renew: %v", err)
	}
	if renewedOK {
		t.Fatalf("expected stale renew to be rejected, got %+v", renewed)
	}

	active, completed, err := registry.CompleteRecovery(ctx, "ROOM01", "instance-b", recovery.Epoch)
	if err != nil {
		t.Fatalf("complete recovery: %v", err)
	}
	if !completed || active.Status != RoomAuthorityStatusActive || active.Epoch != recovery.Epoch {
		t.Fatalf("unexpected completed lease: completed=%t lease=%+v", completed, active)
	}

	refreshed, refreshedOK, err := registry.RenewAuthorityEpoch(ctx, "ROOM01", "instance-b", active.Epoch)
	if err != nil {
		t.Fatalf("renew recovered authority: %v", err)
	}
	if !refreshedOK || refreshed.Epoch != active.Epoch || refreshed.Status != RoomAuthorityStatusActive {
		t.Fatalf("expected recovered authority renew to keep epoch, got ok=%t lease=%+v", refreshedOK, refreshed)
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
	return newTestRoomAuthorityRegistryWithClock(t, time.Now)
}

func newTestRoomAuthorityRegistryWithClock(
	t *testing.T,
	now func() time.Time,
) (*RoomAuthorityRegistry, *miniredis.Miniredis) {
	t.Helper()
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})
	return NewRoomAuthorityRegistryWithClock(&RedisClient{client: client}, time.Minute, now), redisServer
}
