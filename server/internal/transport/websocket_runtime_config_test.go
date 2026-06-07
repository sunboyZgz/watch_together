package transport

import (
	"testing"
	"time"
)

func TestNormalizeWebSocketRuntimeConfigCanDisableSeekRateLimit(t *testing.T) {
	config := normalizeWebSocketRuntimeConfig(WebSocketRuntimeConfig{
		SeekMinInterval: -1 * time.Millisecond,
	})

	if config.SeekMinInterval >= 0 {
		t.Fatalf("expected negative seek min interval to remain disabled, got %s", config.SeekMinInterval)
	}
}
