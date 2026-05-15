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
}

func TestNormalizeWebSocketRuntimeConfigKeepsExplicitLimits(t *testing.T) {
	config := normalizeWebSocketRuntimeConfig(WebSocketRuntimeConfig{
		BroadcastConcurrencyLimit: 12,
		BroadcastTimeout:          2 * time.Second,
		BroadcastEnqueueTimeout:   500 * time.Millisecond,
		ClientOutboxCapacity:      7,
		MaxConnections:            20,
		MaxRoomClients:            3,
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
}
