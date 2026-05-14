package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"gorm.io/gorm"

	"watch_together/server/internal/progress"
)

type PostgresProgressStore struct {
	db *gorm.DB
}

// NewPostgresProgressStore creates the PostgreSQL-backed repository for progress writes.
func NewPostgresProgressStore(db *gorm.DB) *PostgresProgressStore {
	return &PostgresProgressStore{db: db}
}

// UpdateMediaProgress upserts one user's latest progress for one playable episode.
func (s *PostgresProgressStore) UpdateMediaProgress(ctx context.Context, params progress.UpdateParams) (progress.Summary, error) {
	var summary progress.Summary
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if exists, err := rowExists(ctx, tx, `SELECT 1 FROM users WHERE id = ?`, params.UserID); err != nil {
			return fmt.Errorf("check progress user: %w", err)
		} else if !exists {
			return progress.ErrUserNotFound
		}

		episodeID, err := findProgressEpisodeID(ctx, tx, params.MediaItemID)
		if err != nil {
			return err
		}
		if episodeID == "" {
			return progress.ErrMediaNotFound
		}

		existingID, err := findExistingProgressID(ctx, tx, params.UserID, episodeID)
		if err != nil {
			return err
		}
		if existingID != "" {
			summary, err = updateExistingProgress(ctx, tx, existingID, params, episodeID)
			return err
		}
		summary, err = insertProgress(ctx, tx, params, episodeID)
		return err
	})
	if err != nil {
		return progress.Summary{}, err
	}
	return summary, nil
}

func findProgressEpisodeID(ctx context.Context, tx *gorm.DB, mediaItemID string) (string, error) {
	const query = `
		SELECT episode.id::text
		FROM media_episodes AS episode
		INNER JOIN media_seasons AS season ON season.id = episode.season_id
		WHERE episode.id = ?
			AND episode.status = 'active'
			AND season.status = 'active'
		LIMIT 1
	`
	var episodeID string
	if err := tx.WithContext(ctx).Raw(query, mediaItemID).Row().Scan(&episodeID); err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", fmt.Errorf("find progress media: %w", err)
	}
	return episodeID, nil
}

func findExistingProgressID(ctx context.Context, tx *gorm.DB, userID string, episodeID string) (string, error) {
	const query = `
		SELECT id::text
		FROM user_media_progress
		WHERE user_id = ? AND media_episode_id = ?
		LIMIT 1
	`
	var id string
	if err := tx.WithContext(ctx).Raw(query, userID, episodeID).Row().Scan(&id); err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", fmt.Errorf("find existing progress: %w", err)
	}
	return id, nil
}

func updateExistingProgress(ctx context.Context, tx *gorm.DB, progressID string, params progress.UpdateParams, episodeID string) (progress.Summary, error) {
	const query = `
		UPDATE user_media_progress
		SET
			last_position_seconds = ?,
			duration_seconds = ?,
			last_watched_at = NOW(),
			completed = ?,
			completion_source = ?,
			updated_at = NOW()
		WHERE id = ?
		RETURNING media_episode_id::text, last_position_seconds, duration_seconds, completed, last_watched_at
	`

	var summary progress.Summary
	if err := tx.WithContext(ctx).Raw(
		query,
		params.LastPositionSeconds,
		params.DurationSeconds,
		params.Completed,
		params.CompletionSource,
		progressID,
	).Row().Scan(
		&summary.MediaItemID,
		&summary.LastPositionSeconds,
		&summary.DurationSeconds,
		&summary.Completed,
		&summary.LastWatchedAt,
	); err != nil {
		return progress.Summary{}, fmt.Errorf("update media progress: %w", err)
	}

	if summary.MediaItemID == "" {
		summary.MediaItemID = episodeID
	}
	summary.LastWatchedAt = summary.LastWatchedAt.UTC().Truncate(time.Second)
	return summary, nil
}

func insertProgress(ctx context.Context, tx *gorm.DB, params progress.UpdateParams, episodeID string) (progress.Summary, error) {
	const query = `
		INSERT INTO user_media_progress (
			user_id,
			media_episode_id,
			last_position_seconds,
			duration_seconds,
			last_watched_at,
			completed,
			completion_source
		)
		VALUES (?, ?, ?, ?, NOW(), ?, ?)
		RETURNING media_episode_id::text, last_position_seconds, duration_seconds, completed, last_watched_at
	`

	var summary progress.Summary
	if err := tx.WithContext(ctx).Raw(
		query,
		params.UserID,
		episodeID,
		params.LastPositionSeconds,
		params.DurationSeconds,
		params.Completed,
		params.CompletionSource,
	).Row().Scan(
		&summary.MediaItemID,
		&summary.LastPositionSeconds,
		&summary.DurationSeconds,
		&summary.Completed,
		&summary.LastWatchedAt,
	); err != nil {
		return progress.Summary{}, fmt.Errorf("upsert media progress: %w", err)
	}

	summary.LastWatchedAt = summary.LastWatchedAt.UTC().Truncate(time.Second)
	return summary, nil
}
