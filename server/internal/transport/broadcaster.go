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
	EnqueueJSON(ctx context.Context, message any) (room.EnqueueResult, error)
	Close(status websocket.StatusCode, reason string) error
}

type roomBroadcaster interface {
	Broadcast(ctx context.Context, clients []clientWriter, envelope protocol.Envelope) (broadcastStats, error)
}

type broadcastStats struct {
	Clients              int
	FailedClients        int
	TimedOutClients      int
	ClosedClients        int
	CoalescedClients     int
	QueuePressureClients int
	MaxQueueDepth        int
	Duration             time.Duration
	SlowestUserID        string
	SlowestDuration      time.Duration
}

type broadcastConfig struct {
	ConcurrencyLimit      int64
	BroadcastTimeout      time.Duration
	EnqueueTimeout        time.Duration
	CloseOnEnqueueTimeout bool
}

type boundedBroadcaster struct {
	limit                 *semaphore.Weighted
	broadcastTimeout      time.Duration
	enqueueTimeout        time.Duration
	closeOnEnqueueTimeout bool
}

func newBoundedBroadcaster(config broadcastConfig) *boundedBroadcaster {
	if config.ConcurrencyLimit <= 0 {
		config.ConcurrencyLimit = 1
	}
	return &boundedBroadcaster{
		limit:                 semaphore.NewWeighted(config.ConcurrencyLimit),
		broadcastTimeout:      config.BroadcastTimeout,
		enqueueTimeout:        config.EnqueueTimeout,
		closeOnEnqueueTimeout: config.CloseOnEnqueueTimeout,
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
	if b.broadcastTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, b.broadcastTimeout)
		defer cancel()
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
			enqueueCtx := ctx
			cancel := func() {}
			if b.enqueueTimeout > 0 {
				enqueueCtx, cancel = context.WithTimeout(ctx, b.enqueueTimeout)
			}
			defer cancel()

			enqueueResult, err := client.EnqueueJSON(enqueueCtx, envelope)
			clientDuration := time.Since(clientStartedAt)
			enqueueTimedOut := err != nil && isEnqueueTimeout(ctx, enqueueCtx)
			closed := false
			if enqueueTimedOut && b.closeOnEnqueueTimeout {
				closed = client.Close(websocket.StatusPolicyViolation, "broadcast queue timeout") == nil
			}

			mu.Lock()
			defer mu.Unlock()

			if clientDuration > stats.SlowestDuration {
				stats.SlowestDuration = clientDuration
				stats.SlowestUserID = client.UserID()
			}
			if err == nil {
				if enqueueResult.Coalesced {
					stats.CoalescedClients++
				}
				recordQueueStatsLocked(&stats, enqueueResult)
				return
			}

			stats.FailedClients++
			recordQueueStatsLocked(&stats, enqueueResult)
			if firstErr == nil {
				firstErr = err
			}
			if enqueueTimedOut {
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

func isEnqueueTimeout(parentCtx context.Context, enqueueCtx context.Context) bool {
	return parentCtx.Err() == nil && errors.Is(enqueueCtx.Err(), context.DeadlineExceeded)
}

func enqueueQueueUnderPressure(result room.EnqueueResult) bool {
	return result.QueueCapacity > 0 && result.QueueDepth*100 >= result.QueueCapacity*80
}

func recordQueueStatsLocked(stats *broadcastStats, result room.EnqueueResult) {
	if enqueueQueueUnderPressure(result) {
		stats.QueuePressureClients++
	}
	if result.QueueDepth > stats.MaxQueueDepth {
		stats.MaxQueueDepth = result.QueueDepth
	}
}
