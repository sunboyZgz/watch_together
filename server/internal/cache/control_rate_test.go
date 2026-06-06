package cache

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestControlRateRegistryDisabled(t *testing.T) {
	registry := NewControlRateRegistry(nil)

	_, _, err := registry.Reserve(context.Background(), "ROOM01", "seek", time.Second, 1)
	if !errors.Is(err, ErrRedisDisabled) {
		t.Fatalf("expected ErrRedisDisabled, got %v", err)
	}
}

func TestControlRateRegistryReserveAndTTL(t *testing.T) {
	ctx := context.Background()
	registry, redisServer := newTestControlRateRegistry(t)
	defer redisServer.Close()

	reservation, ok, err := registry.Reserve(ctx, "ROOM01", "seek", time.Second, 1)
	if err != nil {
		t.Fatalf("reserve seek: %v", err)
	}
	if !ok || reservation.Token == "" {
		t.Fatalf("expected first reserve to succeed, ok=%t reservation=%+v", ok, reservation)
	}
	if !redisServer.Exists(ControlRateKey("ROOM01", "seek")) {
		t.Fatalf("expected control-rate key %q to exist", ControlRateKey("ROOM01", "seek"))
	}

	_, ok, err = registry.Reserve(ctx, "ROOM01", "seek", time.Second, 1)
	if err != nil {
		t.Fatalf("reserve duplicate seek: %v", err)
	}
	if ok {
		t.Fatalf("expected duplicate reserve to be rate-limited")
	}

	redisServer.FastForward(time.Second + time.Millisecond)
	_, ok, err = registry.Reserve(ctx, "ROOM01", "seek", time.Second, 1)
	if err != nil {
		t.Fatalf("reserve after ttl: %v", err)
	}
	if !ok {
		t.Fatalf("expected reserve after ttl to succeed")
	}
}

func TestControlRateRegistryConcurrentReserveOnlyOneSucceeds(t *testing.T) {
	ctx := context.Background()
	registry, redisServer := newTestControlRateRegistry(t)
	defer redisServer.Close()

	const attempts = 16
	var wg sync.WaitGroup
	results := make(chan bool, attempts)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, ok, err := registry.Reserve(ctx, "ROOM01", "seek", time.Second, 1)
			if err != nil {
				t.Errorf("reserve seek: %v", err)
				results <- false
				return
			}
			results <- ok
		}()
	}
	wg.Wait()
	close(results)

	successes := 0
	for ok := range results {
		if ok {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("expected one successful reserve, got %d", successes)
	}
}

func TestControlRateRegistryReleaseOnlyMatchingReservation(t *testing.T) {
	ctx := context.Background()
	registry, redisServer := newTestControlRateRegistry(t)
	defer redisServer.Close()

	reservation, ok, err := registry.Reserve(ctx, "ROOM01", "seek", time.Second, 1)
	if err != nil || !ok {
		t.Fatalf("reserve seek ok=%t err=%v", ok, err)
	}
	mismatched := reservation
	mismatched.Token = "not-the-token"
	released, err := registry.ReleaseIfMatch(ctx, mismatched)
	if err != nil {
		t.Fatalf("release mismatched token: %v", err)
	}
	if released {
		t.Fatalf("expected mismatched token not to release")
	}
	if !redisServer.Exists(ControlRateKey("ROOM01", "seek")) {
		t.Fatalf("expected control-rate key to remain")
	}

	released, err = registry.ReleaseIfMatch(ctx, reservation)
	if err != nil {
		t.Fatalf("release matching reservation: %v", err)
	}
	if !released || redisServer.Exists(ControlRateKey("ROOM01", "seek")) {
		t.Fatalf("expected matching release to delete control-rate key")
	}
}

func newTestControlRateRegistry(t *testing.T) (*ControlRateRegistry, *miniredis.Miniredis) {
	t.Helper()
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})
	return NewControlRateRegistry(&RedisClient{client: client}), redisServer
}
