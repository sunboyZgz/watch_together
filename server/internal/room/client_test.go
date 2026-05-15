package room

import (
	"context"
	"errors"
	"testing"

	"golang.org/x/sync/semaphore"
)

func TestClientConnectionWriteJSONRespectsContextWhileWaitingForWriteLock(t *testing.T) {
	client := &ClientConnection{
		writeMu: semaphore.NewWeighted(1),
	}
	if err := client.writeMu.Acquire(context.Background(), 1); err != nil {
		t.Fatalf("acquire write lock: %v", err)
	}
	defer client.writeMu.Release(1)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := client.WriteJSON(ctx, map[string]string{"type": "room_state"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
}
