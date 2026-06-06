package transport

import (
	"testing"
	"time"

	"watch_together/server/internal/room"
)

func TestNormalizeWebSocketRuntimeConfigAppliesDefaults(t *testing.T) {
	config := normalizeWebSocketRuntimeConfig(WebSocketRuntimeConfig{})

	if config.BroadcastConcurrencyLimit != defaultBroadcastConcurrencyLimit {
		t.Fatalf("expected default broadcast concurrency, got %d", config.BroadcastConcurrencyLimit)
	}
	if config.BroadcastTimeout != defaultBroadcastTimeout {
		t.Fatalf("expected default broadcast timeout, got %s", config.BroadcastTimeout)
	}
	if config.BroadcastEnqueueTimeout != defaultBroadcastEnqueueTimeout {
		t.Fatalf("expected default enqueue timeout, got %s", config.BroadcastEnqueueTimeout)
	}
	if config.ClientOutboxCapacity != room.DefaultClientOutboxCapacity() {
		t.Fatalf("expected default outbox capacity, got %d", config.ClientOutboxCapacity)
	}
	if config.MaxConnections != 0 {
		t.Fatalf("expected max connections to stay unlimited, got %d", config.MaxConnections)
	}
	if config.MaxRoomClients != 0 {
		t.Fatalf("expected max room clients to stay unlimited, got %d", config.MaxRoomClients)
	}
	if config.SeekMinInterval != defaultSeekMinInterval {
		t.Fatalf("expected default seek min interval, got %s", config.SeekMinInterval)
	}
	if config.ControlIdempotencyTTL != defaultControlIdempotencyTTL {
		t.Fatalf("expected default control idempotency ttl, got %s", config.ControlIdempotencyTTL)
	}
	if config.PresenceLeaseTTL != defaultPresenceLeaseTTL {
		t.Fatalf("expected default presence lease ttl, got %s", config.PresenceLeaseTTL)
	}
	if config.PresenceRefreshInterval != defaultPresenceRefreshInterval {
		t.Fatalf("expected default presence refresh interval, got %s", config.PresenceRefreshInterval)
	}
}

func TestNormalizeWebSocketRuntimeConfigKeepsExplicitLimits(t *testing.T) {
	config := normalizeWebSocketRuntimeConfig(WebSocketRuntimeConfig{
		BroadcastConcurrencyLimit: 12,
		BroadcastTimeout:          2 * time.Second,
		BroadcastEnqueueTimeout:   500 * time.Millisecond,
		ClientOutboxCapacity:      7,
		MaxConnections:            20,
		MaxRoomClients:            3,
		SeekMinInterval:           100 * time.Millisecond,
		ControlIdempotencyTTL:     3 * time.Minute,
		PresenceLeaseTTL:          40 * time.Second,
		PresenceRefreshInterval:   10 * time.Second,
	})

	if config.BroadcastConcurrencyLimit != 12 {
		t.Fatalf("expected explicit broadcast concurrency, got %d", config.BroadcastConcurrencyLimit)
	}
	if config.BroadcastTimeout != 2*time.Second {
		t.Fatalf("expected explicit broadcast timeout, got %s", config.BroadcastTimeout)
	}
	if config.BroadcastEnqueueTimeout != 500*time.Millisecond {
		t.Fatalf("expected explicit enqueue timeout, got %s", config.BroadcastEnqueueTimeout)
	}
	if config.ClientOutboxCapacity != 7 {
		t.Fatalf("expected explicit outbox capacity, got %d", config.ClientOutboxCapacity)
	}
	if config.MaxConnections != 20 {
		t.Fatalf("expected explicit max connections, got %d", config.MaxConnections)
	}
	if config.MaxRoomClients != 3 {
		t.Fatalf("expected explicit max room clients, got %d", config.MaxRoomClients)
	}
	if config.SeekMinInterval != 100*time.Millisecond {
		t.Fatalf("expected explicit seek min interval, got %s", config.SeekMinInterval)
	}
	if config.ControlIdempotencyTTL != 3*time.Minute {
		t.Fatalf("expected explicit control idempotency ttl, got %s", config.ControlIdempotencyTTL)
	}
	if config.PresenceLeaseTTL != 40*time.Second {
		t.Fatalf("expected explicit presence lease ttl, got %s", config.PresenceLeaseTTL)
	}
	if config.PresenceRefreshInterval != 10*time.Second {
		t.Fatalf("expected explicit presence refresh interval, got %s", config.PresenceRefreshInterval)
	}
}

func TestNormalizeWebSocketRuntimeConfigCanDisableSeekRateLimit(t *testing.T) {
	config := normalizeWebSocketRuntimeConfig(WebSocketRuntimeConfig{
		SeekMinInterval: -1 * time.Millisecond,
	})

	if config.SeekMinInterval >= 0 {
		t.Fatalf("expected negative seek min interval to remain disabled, got %s", config.SeekMinInterval)
	}
}
