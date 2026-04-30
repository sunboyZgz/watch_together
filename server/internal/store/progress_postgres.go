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

	mediaRef, err := findProgressMedia(ctx, tx, params.MediaItemID)
	if err != nil {
		return progress.Summary{}, err
	}
	if mediaRef.episodeID == "" {
		return progress.Summary{}, progress.ErrMediaNotFound
	}

	existingID, err := findExistingProgressID(ctx, tx, params.UserID, mediaRef.episodeID)
	if err != nil {
		return progress.Summary{}, err
	}
	if existingID != "" {
		return updateExistingProgress(ctx, tx, existingID, params, mediaRef.episodeID)
	}
	return insertProgress(ctx, tx, params, mediaRef)
}

type progressMediaRef struct {
	episodeID         string
	legacyMediaItemID *string
}

func findProgressMedia(ctx context.Context, tx *sql.Tx, mediaItemID string) (progressMediaRef, error) {
	const query = `
		SELECT episode.id::text, episode.legacy_media_item_id::text
		FROM media_episodes AS episode
		INNER JOIN media_seasons AS season ON season.id = episode.season_id
		WHERE (episode.id = $1 OR episode.legacy_media_item_id = $1)
			AND episode.status = 'active'
			AND season.status = 'active'
		ORDER BY CASE WHEN episode.id = $1 THEN 0 ELSE 1 END
		LIMIT 1
	`
	var ref progressMediaRef
	var legacyMediaItemID sql.NullString
	if err := tx.QueryRowContext(ctx, query, mediaItemID).Scan(
		&ref.episodeID,
		&legacyMediaItemID,
	); err != nil {
		if err == sql.ErrNoRows {
			return progressMediaRef{}, nil
		}
		return progressMediaRef{}, fmt.Errorf("find progress media: %w", err)
	}
	ref.legacyMediaItemID = nullableStringPtr(legacyMediaItemID)
	return ref, nil
}

func findExistingProgressID(ctx context.Context, tx *sql.Tx, userID string, episodeID string) (string, error) {
	const query = `
		SELECT id::text
		FROM user_media_progress
		WHERE user_id = $1 AND media_episode_id = $2
		LIMIT 1
	`
	var id string
	if err := tx.QueryRowContext(ctx, query, userID, episodeID).Scan(&id); err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", fmt.Errorf("find existing progress: %w", err)
	}
	return id, nil
}

func updateExistingProgress(ctx context.Context, tx *sql.Tx, progressID string, params progress.UpdateParams, episodeID string) (progress.Summary, error) {
	const query = `
		UPDATE user_media_progress
		SET
			last_position_seconds = $2,
			duration_seconds = $3,
			last_watched_at = NOW(),
			completed = $4,
			completion_source = $5,
			updated_at = NOW()
		WHERE id = $1
		RETURNING media_episode_id::text, last_position_seconds, duration_seconds, completed, last_watched_at
	`

	var summary progress.Summary
	if err := tx.QueryRowContext(
		ctx,
		query,
		progressID,
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
		return progress.Summary{}, fmt.Errorf("update media progress: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return progress.Summary{}, fmt.Errorf("commit update progress: %w", err)
	}
	if summary.MediaItemID == "" {
		summary.MediaItemID = episodeID
	}
	summary.LastWatchedAt = summary.LastWatchedAt.UTC().Truncate(time.Second)
	return summary, nil
}

func insertProgress(ctx context.Context, tx *sql.Tx, params progress.UpdateParams, mediaRef progressMediaRef) (progress.Summary, error) {
	const query = `
		INSERT INTO user_media_progress (
			user_id,
			media_item_id,
			media_episode_id,
			last_position_seconds,
			duration_seconds,
			last_watched_at,
			completed,
			completion_source
		)
		VALUES ($1, $2, $3, $4, $5, NOW(), $6, $7)
		RETURNING media_episode_id::text, last_position_seconds, duration_seconds, completed, last_watched_at
	`

	var summary progress.Summary
	if err := tx.QueryRowContext(
		ctx,
		query,
		params.UserID,
		mediaRef.legacyMediaItemID,
		mediaRef.episodeID,
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
