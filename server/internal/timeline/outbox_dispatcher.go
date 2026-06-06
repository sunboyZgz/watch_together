package timeline

import (
	"context"
	"log"
	"time"
)

type OutboxStore interface {
	ClaimPending(ctx context.Context, batchSize int, now time.Time) ([]OutboxRow, error)
	MarkPublished(ctx context.Context, id string) error
	MarkPublishFailed(ctx context.Context, id string, attempts int, lastError string, nextAttemptAt time.Time) error
}

type OutboxRow struct {
	ID       string
	Topic    string
	EventID  string
	RoomID   string
	Payload  []byte
	Attempts int
}

type OutboxDispatcher struct {
	store        OutboxStore
	publisher    Publisher
	batchSize    int
	pollInterval time.Duration
	now          func() time.Time
	observer     WorkerObserver
}

type WorkerObserver interface {
	RecordWorkerEvent(worker string, result string)
}

func NewOutboxDispatcher(
	store OutboxStore,
	publisher Publisher,
	batchSize int,
	pollInterval time.Duration,
) *OutboxDispatcher {
	if batchSize <= 0 {
		batchSize = 50
	}
	if pollInterval <= 0 {
		pollInterval = time.Second
	}
	return &OutboxDispatcher{
		store:        store,
		publisher:    publisher,
		batchSize:    batchSize,
		pollInterval: pollInterval,
		now:          time.Now,
	}
}

func (d *OutboxDispatcher) SetObserver(observer WorkerObserver) {
	if d == nil {
		return
	}
	d.observer = observer
}

func (d *OutboxDispatcher) Run(ctx context.Context) error {
	if d == nil || d.store == nil || d.publisher == nil {
		return nil
	}
	ticker := time.NewTicker(d.pollInterval)
	defer ticker.Stop()
	for {
		if err := d.DispatchOnce(ctx); err != nil {
			log.Printf("timeline outbox dispatch failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (d *OutboxDispatcher) DispatchOnce(ctx context.Context) error {
	rows, err := d.store.ClaimPending(ctx, d.batchSize, d.now())
	if err != nil {
		return err
	}
	for _, row := range rows {
		if err := d.publisher.Publish(ctx, row.Topic, []byte(row.RoomID), row.Payload); err != nil {
			d.recordWorkerEvent("outboxworker", "publish_failed")
			nextAttempts := row.Attempts + 1
			nextAttemptAt := d.now().Add(backoff(nextAttempts))
			if markErr := d.store.MarkPublishFailed(ctx, row.ID, nextAttempts, err.Error(), nextAttemptAt); markErr != nil {
				log.Printf("timeline outbox mark failed id=%s err=%v", row.ID, markErr)
			}
			continue
		}
		if err := d.store.MarkPublished(ctx, row.ID); err != nil {
			d.recordWorkerEvent("outboxworker", "mark_published_failed")
			log.Printf("timeline outbox mark published failed id=%s err=%v", row.ID, err)
			continue
		}
		d.recordWorkerEvent("outboxworker", "published")
	}
	return nil
}

func (d *OutboxDispatcher) recordWorkerEvent(worker string, result string) {
	if d != nil && d.observer != nil {
		d.observer.RecordWorkerEvent(worker, result)
	}
}

func backoff(attempts int) time.Duration {
	if attempts <= 0 {
		return time.Second
	}
	delay := time.Duration(attempts*attempts) * time.Second
	if delay > time.Minute {
		return time.Minute
	}
	return delay
}
