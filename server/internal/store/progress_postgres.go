package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"watch_together/server/internal/progress"
)

type PostgresProgressStore struct {
	db *sql.DB
}

// NewPostgresProgressStore creates the PostgreSQL-backed repository for progress writes.
func NewPostgresProgressStore(db *sql.DB) *PostgresProgressStore {
	return &PostgresProgressStore{db: db}
}

// UpdateMediaProgress upserts one user's latest progress for one media item.
func (s *PostgresProgressStore) UpdateMediaProgress(ctx context.Context, params progress.UpdateParams) (progress.Summary, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return progress.Summary{}, fmt.Errorf("begin update progress: %w", err)
	}
	defer rollbackTx(tx)

	if exists, err := rowExists(ctx, tx, `SELECT 1 FROM users WHERE id = $1`, params.UserID); err != nil {
		return progress.Summary{}, fmt.Errorf("check progress user: %w", err)
	} else if !exists {
		return progress.Summary{}, progress.ErrUserNotFound
	}

	if exists, err := rowExists(ctx, tx, `SELECT 1 FROM media_items WHERE id = $1 AND status = 'active'`, params.MediaItemID); err != nil {
		return progress.Summary{}, fmt.Errorf("check progress media: %w", err)
	} else if !exists {
		return progress.Summary{}, progress.ErrMediaNotFound
	}

	const query = `
		INSERT INTO user_media_progress (
			user_id,
			media_item_id,
			last_position_seconds,
			duration_seconds,
			last_watched_at,
			completed,
			completion_source
		)
		VALUES ($1, $2, $3, $4, NOW(), $5, $6)
		ON CONFLICT (user_id, media_item_id) DO UPDATE SET
			last_position_seconds = EXCLUDED.last_position_seconds,
			duration_seconds = EXCLUDED.duration_seconds,
			last_watched_at = EXCLUDED.last_watched_at,
			completed = EXCLUDED.completed,
			completion_source = EXCLUDED.completion_source,
			updated_at = NOW()
		RETURNING media_item_id::text, last_position_seconds, duration_seconds, completed, last_watched_at
	`

	var summary progress.Summary
	if err := tx.QueryRowContext(
		ctx,
		query,
		params.UserID,
		params.MediaItemID,
		params.LastPositionSeconds,
		params.DurationSeconds,
		params.Completed,
		params.CompletionSource,
	).Scan(
		&summary.MediaItemID,
		&summary.LastPositionSeconds,
		&summary.DurationSeconds,
		&summary.Completed,
		&summary.LastWatchedAt,
	); err != nil {
		return progress.Summary{}, fmt.Errorf("upsert media progress: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return progress.Summary{}, fmt.Errorf("commit update progress: %w", err)
	}
	summary.LastWatchedAt = summary.LastWatchedAt.UTC().Truncate(time.Second)
	return summary, nil
}
