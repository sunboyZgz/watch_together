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

func TestClientConnectionEnqueueJSONRespectsContextWhenOutboxIsFull(t *testing.T) {
	client := &ClientConnection{
		writeMu: semaphore.NewWeighted(1),
		outbox:  newClientOutbox(1),
	}
	if err := client.EnqueueJSON(context.Background(), map[string]string{"type": "room_state"}); err != nil {
		t.Fatalf("fill outbox: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := client.EnqueueJSON(ctx, map[string]string{"type": "room_state"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
}

func TestClientConnectionEnqueueJSONCoalescesLatestMessage(t *testing.T) {
	client := &ClientConnection{
		writeMu: semaphore.NewWeighted(1),
		outbox:  newClientOutbox(2),
	}

	if err := client.EnqueueJSON(context.Background(), testCoalescableMessage{key: "room_state", value: "old"}); err != nil {
		t.Fatalf("enqueue old state: %v", err)
	}
	if err := client.EnqueueJSON(context.Background(), map[string]string{"type": "room_members_changed"}); err != nil {
		t.Fatalf("enqueue non-coalescable message: %v", err)
	}
	if err := client.EnqueueJSON(context.Background(), testCoalescableMessage{key: "room_state", value: "new"}); err != nil {
		t.Fatalf("enqueue new state: %v", err)
	}

	if size := client.outboxSize(); size != 2 {
		t.Fatalf("expected queue size 2, got %d", size)
	}

	first, err := client.outbox.dequeue(context.Background())
	if err != nil {
		t.Fatalf("dequeue first: %v", err)
	}
	if _, ok := first.message.(map[string]string); !ok {
		t.Fatalf("expected non-coalescable message first, got %T", first.message)
	}

	second, err := client.outbox.dequeue(context.Background())
	if err != nil {
		t.Fatalf("dequeue second: %v", err)
	}
	state, ok := second.message.(testCoalescableMessage)
	if !ok {
		t.Fatalf("expected coalesced state message, got %T", second.message)
	}
	if state.value != "new" {
		t.Fatalf("expected latest state, got %s", state.value)
	}
}

type testCoalescableMessage struct {
	key   string
	value string
}

func (m testCoalescableMessage) OutboxCoalesceKey() string {
	return m.key
}

func (c *ClientConnection) outboxSize() int {
	c.outbox.mu.Lock()
	defer c.outbox.mu.Unlock()

	return len(c.outbox.queue)
}
