package transport

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/coder/websocket"
	"golang.org/x/sync/semaphore"

	"watch_together/server/internal/protocol"
	"watch_together/server/internal/room"
)

type clientWriter interface {
	UserID() string
	WriteJSON(ctx context.Context, message any) error
	Close(status websocket.StatusCode, reason string) error
}

type roomBroadcaster interface {
	Broadcast(ctx context.Context, clients []clientWriter, envelope protocol.Envelope) (broadcastStats, error)
}

type broadcastStats struct {
	Clients         int
	FailedClients   int
	TimedOutClients int
	ClosedClients   int
	Duration        time.Duration
	SlowestUserID   string
	SlowestDuration time.Duration
}

type broadcastConfig struct {
	ConcurrencyLimit    int64
	WriteTimeout       time.Duration
	CloseOnWriteTimeout bool
}

type boundedBroadcaster struct {
	limit               *semaphore.Weighted
	writeTimeout        time.Duration
	closeOnWriteTimeout bool
}

func newBoundedBroadcaster(config broadcastConfig) *boundedBroadcaster {
	if config.ConcurrencyLimit <= 0 {
		config.ConcurrencyLimit = 1
	}
	return &boundedBroadcaster{
		limit:               semaphore.NewWeighted(config.ConcurrencyLimit),
		writeTimeout:        config.WriteTimeout,
		closeOnWriteTimeout: config.CloseOnWriteTimeout,
	}
}

func (b *boundedBroadcaster) Broadcast(
	ctx context.Context,
	clients []clientWriter,
	envelope protocol.Envelope,
) (broadcastStats, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	startedAt := time.Now()
	targets := compactClientWriters(clients)
	stats := broadcastStats{Clients: len(targets)}
	if len(targets) == 0 {
		stats.Duration = time.Since(startedAt)
		return stats, nil
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error

	for i, client := range targets {
		if err := b.limit.Acquire(ctx, 1); err != nil {
			mu.Lock()
			stats.FailedClients += len(targets) - i
			if firstErr == nil {
				firstErr = err
			}
			mu.Unlock()
			break
		}

		client := client
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer b.limit.Release(1)

			clientStartedAt := time.Now()
			writeCtx := ctx
			cancel := func() {}
			if b.writeTimeout > 0 {
				writeCtx, cancel = context.WithTimeout(ctx, b.writeTimeout)
			}
			defer cancel()

			err := client.WriteJSON(writeCtx, envelope)
			clientDuration := time.Since(clientStartedAt)
			writeTimedOut := err != nil && isWriteTimeout(ctx, writeCtx)
			closed := false
			if writeTimedOut && b.closeOnWriteTimeout {
				closed = client.Close(websocket.StatusPolicyViolation, "broadcast write timeout") == nil
			}

			mu.Lock()
			defer mu.Unlock()

			if clientDuration > stats.SlowestDuration {
				stats.SlowestDuration = clientDuration
				stats.SlowestUserID = client.UserID()
			}
			if err == nil {
				return
			}

			stats.FailedClients++
			if firstErr == nil {
				firstErr = err
			}
			if writeTimedOut {
				stats.TimedOutClients++
				if closed {
					stats.ClosedClients++
				}
			}
		}()
	}

	wg.Wait()
	stats.Duration = time.Since(startedAt)
	return stats, firstErr
}

func compactClientWriters(clients []clientWriter) []clientWriter {
	targets := make([]clientWriter, 0, len(clients))
	for _, client := range clients {
		if client == nil {
			continue
		}
		targets = append(targets, client)
	}
	return targets
}

func roomClientWriters(clients []*room.ClientConnection) []clientWriter {
	writers := make([]clientWriter, 0, len(clients))
	for _, client := range clients {
		if client == nil {
			continue
		}
		writers = append(writers, client)
	}
	return writers
}

func isWriteTimeout(parentCtx context.Context, writeCtx context.Context) bool {
	return parentCtx.Err() == nil && errors.Is(writeCtx.Err(), context.DeadlineExceeded)
}
