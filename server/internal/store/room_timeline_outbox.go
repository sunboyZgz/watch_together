package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"

	"watch_together/server/internal/timeline"
)

type PostgresTimelineOutboxStore struct {
	db             *gorm.DB
	canonicalTopic string
	now            func() time.Time
}

func NewPostgresTimelineOutboxStore(db *gorm.DB, canonicalTopic string) *PostgresTimelineOutboxStore {
	if canonicalTopic == "" {
		canonicalTopic = timeline.DefaultCanonicalTopic
	}
	return &PostgresTimelineOutboxStore{
		db:             db,
		canonicalTopic: canonicalTopic,
		now:            time.Now,
	}
}

func (s *PostgresTimelineOutboxStore) RecordTimelineEvent(ctx context.Context, event timeline.Event) error {
	if s == nil || s.db == nil {
		return nil
	}
	if event.EventID == "" {
		event.EventID = timeline.NewEventID()
	}
	if event.EventVersion == 0 {
		event.EventVersion = timeline.EventVersion
	}
	if event.OccurredAtMs == 0 {
		event.OccurredAtMs = s.now().UnixMilli()
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal timeline event: %w", err)
	}
	const query = `
		INSERT INTO room_timeline_outbox (
			topic,
			event_id,
			event_type,
			room_id,
			payload,
			status,
			next_attempt_at
		)
		VALUES (?, ?, ?, ?, ?::jsonb, 'pending', NOW())
	`
	if err := s.db.WithContext(ctx).Exec(
		query,
		s.canonicalTopic,
		event.EventID,
		event.EventType,
		event.RoomID,
		string(payload),
	).Error; err != nil {
		return fmt.Errorf("insert timeline outbox: %w", err)
	}
	return nil
}

func (s *PostgresTimelineOutboxStore) ClaimPending(
	ctx context.Context,
	batchSize int,
	now time.Time,
) ([]timeline.OutboxRow, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	if batchSize <= 0 {
		batchSize = 50
	}
	if now.IsZero() {
		now = s.now()
	}
	rows := make([]timeline.OutboxRow, 0, batchSize)
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		const query = `
			SELECT
				id::text,
				topic,
				event_id,
				room_id,
				payload::text,
				attempts
			FROM room_timeline_outbox
			WHERE status = 'pending'
				AND next_attempt_at <= ?
			ORDER BY created_at ASC
			LIMIT ?
			FOR UPDATE SKIP LOCKED
		`
		sqlRows, err := tx.Raw(query, now, batchSize).Rows()
		if err != nil {
			return fmt.Errorf("claim timeline outbox: %w", err)
		}
		defer sqlRows.Close()
		ids := make([]string, 0, batchSize)
		for sqlRows.Next() {
			var row timeline.OutboxRow
			var payloadText string
			if err := sqlRows.Scan(
				&row.ID,
				&row.Topic,
				&row.EventID,
				&row.RoomID,
				&payloadText,
				&row.Attempts,
			); err != nil {
				return fmt.Errorf("scan timeline outbox: %w", err)
			}
			row.Payload = []byte(payloadText)
			rows = append(rows, row)
			ids = append(ids, row.ID)
		}
		if err := sqlRows.Err(); err != nil {
			return fmt.Errorf("iterate timeline outbox: %w", err)
		}
		if len(ids) == 0 {
			return nil
		}
		if err := tx.Exec(
			`UPDATE room_timeline_outbox SET status = 'publishing', updated_at = NOW() WHERE id IN ?`,
			ids,
		).Error; err != nil {
			return fmt.Errorf("mark timeline outbox publishing: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *PostgresTimelineOutboxStore) MarkPublished(ctx context.Context, id string) error {
	if s == nil || s.db == nil || id == "" {
		return nil
	}
	const query = `
		UPDATE room_timeline_outbox
		SET status = 'published',
			published_at = NOW(),
			updated_at = NOW()
		WHERE id = ?
	`
	if err := s.db.WithContext(ctx).Exec(query, id).Error; err != nil {
		return fmt.Errorf("mark timeline outbox published: %w", err)
	}
	return nil
}

func (s *PostgresTimelineOutboxStore) MarkPublishFailed(
	ctx context.Context,
	id string,
	attempts int,
	lastError string,
	nextAttemptAt time.Time,
) error {
	if s == nil || s.db == nil || id == "" {
		return nil
	}
	if attempts < 0 {
		attempts = 0
	}
	if nextAttemptAt.IsZero() {
		nextAttemptAt = s.now().Add(time.Second)
	}
	const query = `
		UPDATE room_timeline_outbox
		SET status = 'pending',
			attempts = ?,
			last_error = ?,
			next_attempt_at = ?,
			updated_at = NOW()
		WHERE id = ?
	`
	if err := s.db.WithContext(ctx).Exec(query, attempts, lastError, nextAttemptAt, id).Error; err != nil {
		return fmt.Errorf("mark timeline outbox failed: %w", err)
	}
	return nil
}
