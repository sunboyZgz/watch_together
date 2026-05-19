package transport

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"watch_together/server/internal/protocol"
	"watch_together/server/internal/room"
)

func TestBoundedBroadcasterSkipsNilClients(t *testing.T) {
	broadcaster := newBoundedBroadcaster(broadcastConfig{
		ConcurrencyLimit:      2,
		EnqueueTimeout:        time.Second,
		CloseOnEnqueueTimeout: true,
	})
	client := &fakeBroadcastClient{userID: "user_a"}

	stats, err := broadcaster.Broadcast(context.Background(), []clientWriter{nil, client}, protocol.Envelope{
		Type: protocol.TypeRoomState,
	})

	if err != nil {
		t.Fatalf("broadcast: %v", err)
	}
	if stats.Clients != 1 {
		t.Fatalf("expected 1 client, got %d", stats.Clients)
	}
	if enqueues := client.enqueueCount(); enqueues != 1 {
		t.Fatalf("expected 1 enqueue, got %d", enqueues)
	}
	if stats.MaxQueueDepth != 1 {
		t.Fatalf("expected max queue depth 1, got %d", stats.MaxQueueDepth)
	}
}

func TestBoundedBroadcasterReportsEnqueueFailure(t *testing.T) {
	expectedErr := errors.New("enqueue failed")
	broadcaster := newBoundedBroadcaster(broadcastConfig{
		ConcurrencyLimit:      2,
		EnqueueTimeout:        time.Second,
		CloseOnEnqueueTimeout: true,
	})
	client := &fakeBroadcastClient{
		userID:     "user_a",
		enqueueErr: expectedErr,
	}

	stats, err := broadcaster.Broadcast(context.Background(), []clientWriter{client}, protocol.Envelope{
		Type: protocol.TypeRoomState,
	})

	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected enqueue error, got %v", err)
	}
	if stats.FailedClients != 1 {
		t.Fatalf("expected 1 failed client, got %d", stats.FailedClients)
	}
}

func TestBoundedBroadcasterClosesTimedOutClient(t *testing.T) {
	broadcaster := newBoundedBroadcaster(broadcastConfig{
		ConcurrencyLimit:      1,
		EnqueueTimeout:        time.Millisecond,
		CloseOnEnqueueTimeout: true,
	})
	client := &fakeBroadcastClient{
		userID:        "user_a",
		blockUntilCtx: true,
		enqueueResult: room.EnqueueResult{
			QueueDepth:    10,
			QueueCapacity: 10,
		},
	}

	stats, err := broadcaster.Broadcast(context.Background(), []clientWriter{client}, protocol.Envelope{
		Type: protocol.TypeRoomState,
	})

	if err == nil {
		t.Fatalf("expected timeout error")
	}
	if stats.TimedOutClients != 1 {
		t.Fatalf("expected 1 timed out client, got %d", stats.TimedOutClients)
	}
	if stats.ClosedClients != 1 {
		t.Fatalf("expected 1 closed client, got %d", stats.ClosedClients)
	}
	if stats.QueuePressureClients != 1 {
		t.Fatalf("expected 1 queue pressure client, got %d", stats.QueuePressureClients)
	}
	if stats.MaxQueueDepth != 10 {
		t.Fatalf("expected max queue depth 10, got %d", stats.MaxQueueDepth)
	}
	if !client.isClosed() {
		t.Fatalf("expected client to be closed")
	}
}

func TestBoundedBroadcasterReportsCoalescedClientsAndMaxQueueDepth(t *testing.T) {
	broadcaster := newBoundedBroadcaster(broadcastConfig{
		ConcurrencyLimit:      2,
		EnqueueTimeout:        time.Second,
		CloseOnEnqueueTimeout: true,
	})
	clients := []clientWriter{
		&fakeBroadcastClient{
			userID: "user_a",
			enqueueResult: room.EnqueueResult{
				QueueDepth: 3,
				Coalesced:  true,
			},
		},
		&fakeBroadcastClient{
			userID: "user_b",
			enqueueResult: room.EnqueueResult{
				QueueDepth:    5,
				QueueCapacity: 8,
			},
		},
	}

	stats, err := broadcaster.Broadcast(context.Background(), clients, protocol.Envelope{
		Type: protocol.TypeRoomState,
	})

	if err != nil {
		t.Fatalf("broadcast: %v", err)
	}
	if stats.CoalescedClients != 1 {
		t.Fatalf("expected 1 coalesced client, got %d", stats.CoalescedClients)
	}
	if stats.MaxQueueDepth != 5 {
		t.Fatalf("expected max queue depth 5, got %d", stats.MaxQueueDepth)
	}
}

func TestBoundedBroadcasterReportsQueuePressureClients(t *testing.T) {
	broadcaster := newBoundedBroadcaster(broadcastConfig{
		ConcurrencyLimit:      2,
		EnqueueTimeout:        time.Second,
		CloseOnEnqueueTimeout: true,
	})
	clients := []clientWriter{
		&fakeBroadcastClient{
			userID: "user_a",
			enqueueResult: room.EnqueueResult{
				QueueDepth:    8,
				QueueCapacity: 10,
			},
		},
		&fakeBroadcastClient{
			userID: "user_b",
			enqueueResult: room.EnqueueResult{
				QueueDepth:    7,
				QueueCapacity: 10,
			},
		},
	}

	stats, err := broadcaster.Broadcast(context.Background(), clients, protocol.Envelope{
		Type: protocol.TypeRoomState,
	})

	if err != nil {
		t.Fatalf("broadcast: %v", err)
	}
	if stats.QueuePressureClients != 1 {
		t.Fatalf("expected 1 queue pressure client, got %d", stats.QueuePressureClients)
	}
}

func TestBoundedBroadcasterStopsSchedulingAfterBroadcastTimeout(t *testing.T) {
	broadcaster := newBoundedBroadcaster(broadcastConfig{
		ConcurrencyLimit:      1,
		BroadcastTimeout:      time.Millisecond,
		EnqueueTimeout:        time.Second,
		CloseOnEnqueueTimeout: true,
	})
	clients := []clientWriter{
		&fakeBroadcastClient{userID: "user_a", blockUntilCtx: true},
		&fakeBroadcastClient{userID: "user_b"},
		&fakeBroadcastClient{userID: "user_c"},
	}

	stats, err := broadcaster.Broadcast(context.Background(), clients, protocol.Envelope{
		Type: protocol.TypeRoomState,
	})

	if err == nil {
		t.Fatalf("expected broadcast timeout error")
	}
	if stats.FailedClients != len(clients) {
		t.Fatalf("expected all clients to be failed, got %d", stats.FailedClients)
	}
}

func TestBoundedBroadcasterHonorsConcurrencyLimit(t *testing.T) {
	broadcaster := newBoundedBroadcaster(broadcastConfig{
		ConcurrencyLimit:      2,
		EnqueueTimeout:        time.Second,
		CloseOnEnqueueTimeout: true,
	})

	release := make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	entered := make(chan struct{}, 4)
	var mu sync.Mutex
	active := 0
	maxActive := 0
	clients := make([]clientWriter, 0, 4)
	for i := 0; i < 4; i++ {
		clients = append(clients, &fakeBroadcastClient{
			userID:     "user",
			blockUntil: release,
			onEnqueue: func() {
				mu.Lock()
				defer mu.Unlock()
				active++
				if active > maxActive {
					maxActive = active
				}
				entered <- struct{}{}
			},
			afterEnqueue: func() {
				mu.Lock()
				defer mu.Unlock()
				active--
			},
		})
	}

	done := make(chan error, 1)
	go func() {
		_, err := broadcaster.Broadcast(context.Background(), clients, protocol.Envelope{
			Type: protocol.TypeRoomState,
		})
		done <- err
	}()

	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for initial enqueues")
		}
	}
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	observedMax := maxActive
	mu.Unlock()
	if observedMax > 2 {
		t.Fatalf("expected at most 2 active enqueues, got %d", observedMax)
	}

	close(release)
	released = true
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("broadcast: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for broadcast")
	}
}

type fakeBroadcastClient struct {
	userID        string
	enqueues      int
	enqueueResult room.EnqueueResult
	enqueueErr    error
	blockUntilCtx bool
	blockUntil    <-chan struct{}
	closed        bool
	onEnqueue     func()
	afterEnqueue  func()
	mu            sync.Mutex
}

func (c *fakeBroadcastClient) UserID() string {
	return c.userID
}

func (c *fakeBroadcastClient) EnqueueJSON(ctx context.Context, message any) (room.EnqueueResult, error) {
	c.mu.Lock()
	c.enqueues++
	enqueues := c.enqueues
	enqueueResult := c.enqueueResult
	c.mu.Unlock()

	if c.onEnqueue != nil {
		c.onEnqueue()
	}
	if c.afterEnqueue != nil {
		defer c.afterEnqueue()
	}
	if c.blockUntilCtx {
		<-ctx.Done()
		return enqueueResult, ctx.Err()
	}
	if c.blockUntil != nil {
		select {
		case <-ctx.Done():
			return enqueueResult, ctx.Err()
		case <-c.blockUntil:
		}
	}
	if c.enqueueErr != nil {
		return room.EnqueueResult{}, c.enqueueErr
	}
	if enqueueResult.QueueDepth == 0 {
		enqueueResult.QueueDepth = enqueues
	}
	return enqueueResult, nil
}

func (c *fakeBroadcastClient) Close(status websocket.StatusCode, reason string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.closed = true
	return nil
}

func (c *fakeBroadcastClient) enqueueCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.enqueues
}

func (c *fakeBroadcastClient) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.closed
}
