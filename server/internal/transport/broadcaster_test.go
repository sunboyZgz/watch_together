package transport

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"watch_together/server/internal/protocol"
)

func TestBoundedBroadcasterSkipsNilClients(t *testing.T) {
	broadcaster := newBoundedBroadcaster(broadcastConfig{
		ConcurrencyLimit:    2,
		WriteTimeout:        time.Second,
		CloseOnWriteTimeout: true,
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
	if writes := client.writeCount(); writes != 1 {
		t.Fatalf("expected 1 write, got %d", writes)
	}
}

func TestBoundedBroadcasterReportsWriteFailure(t *testing.T) {
	expectedErr := errors.New("write failed")
	broadcaster := newBoundedBroadcaster(broadcastConfig{
		ConcurrencyLimit:    2,
		WriteTimeout:        time.Second,
		CloseOnWriteTimeout: true,
	})
	client := &fakeBroadcastClient{
		userID:   "user_a",
		writeErr: expectedErr,
	}

	stats, err := broadcaster.Broadcast(context.Background(), []clientWriter{client}, protocol.Envelope{
		Type: protocol.TypeRoomState,
	})

	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected write error, got %v", err)
	}
	if stats.FailedClients != 1 {
		t.Fatalf("expected 1 failed client, got %d", stats.FailedClients)
	}
}

func TestBoundedBroadcasterClosesTimedOutClient(t *testing.T) {
	broadcaster := newBoundedBroadcaster(broadcastConfig{
		ConcurrencyLimit:    1,
		WriteTimeout:        time.Millisecond,
		CloseOnWriteTimeout: true,
	})
	client := &fakeBroadcastClient{
		userID:       "user_a",
		blockUntilCtx: true,
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
	if !client.isClosed() {
		t.Fatalf("expected client to be closed")
	}
}

func TestBoundedBroadcasterHonorsConcurrencyLimit(t *testing.T) {
	broadcaster := newBoundedBroadcaster(broadcastConfig{
		ConcurrencyLimit:    2,
		WriteTimeout:        time.Second,
		CloseOnWriteTimeout: true,
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
			onWrite: func() {
				mu.Lock()
				defer mu.Unlock()
				active++
				if active > maxActive {
					maxActive = active
				}
				entered <- struct{}{}
			},
			afterWrite: func() {
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
			t.Fatalf("timed out waiting for initial writes")
		}
	}
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	observedMax := maxActive
	mu.Unlock()
	if observedMax > 2 {
		t.Fatalf("expected at most 2 active writes, got %d", observedMax)
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
	writes        int
	writeErr      error
	blockUntilCtx bool
	blockUntil    <-chan struct{}
	closed        bool
	onWrite       func()
	afterWrite    func()
	mu            sync.Mutex
}

func (c *fakeBroadcastClient) UserID() string {
	return c.userID
}

func (c *fakeBroadcastClient) WriteJSON(ctx context.Context, message any) error {
	c.mu.Lock()
	c.writes++
	c.mu.Unlock()

	if c.onWrite != nil {
		c.onWrite()
	}
	if c.afterWrite != nil {
		defer c.afterWrite()
	}
	if c.blockUntilCtx {
		<-ctx.Done()
		return ctx.Err()
	}
	if c.blockUntil != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.blockUntil:
		}
	}
	return c.writeErr
}

func (c *fakeBroadcastClient) Close(status websocket.StatusCode, reason string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.closed = true
	return nil
}

func (c *fakeBroadcastClient) writeCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.writes
}

func (c *fakeBroadcastClient) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.closed
}
