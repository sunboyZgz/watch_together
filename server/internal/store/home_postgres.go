package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"watch_together/server/internal/home"
)

type PostgresHomeStore struct {
	db *gorm.DB
}

// NewPostgresHomeStore creates the PostgreSQL-backed repository for home data.
func NewPostgresHomeStore(db *gorm.DB) *PostgresHomeStore {
	return &PostgresHomeStore{db: db}
}

// GetHomeSummary loads the profile, latest progress, and continue-watching rows.
func (s *PostgresHomeStore) GetHomeSummary(ctx context.Context, userID string) (home.Summary, error) {
	user, err := s.findHomeUser(ctx, userID)
	if err != nil {
		return home.Summary{}, err
	}

	lastWatched, err := s.findLastWatched(ctx, userID)
	if err != nil {
		return home.Summary{}, err
	}

	continueWatching, err := s.findContinueWatching(ctx, userID, 2)
	if err != nil {
		return home.Summary{}, err
	}

	return home.Summary{
		User:             user,
		LastWatched:      lastWatched,
		ContinueWatching: continueWatching,
	}, nil
}

func (s *PostgresHomeStore) findHomeUser(ctx context.Context, userID string) (home.UserSummary, error) {
	const query = `
		SELECT nickname, avatar_seed, avatar_url
		FROM users
		WHERE id = ?
	`

	var user home.UserSummary
	var avatarURL sql.NullString
	if err := s.db.WithContext(ctx).Raw(query, userID).Row().Scan(
		&user.Nickname,
		&user.AvatarSeed,
		&avatarURL,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return home.UserSummary{}, home.ErrUserNotFound
		}
		return home.UserSummary{}, fmt.Errorf("find home user: %w", err)
	}
	if avatarURL.Valid {
		user.AvatarURL = &avatarURL.String
	}
	return user, nil
}

func (s *PostgresHomeStore) findLastWatched(ctx context.Context, userID string) (*home.WatchProgressSummary, error) {
	const query = `
		SELECT
			episode.id::text,
			season.title,
			COALESCE(episode.cover_url, season.cover_url),
			progress.last_position_seconds,
			progress.duration_seconds
		FROM user_media_progress AS progress
		INNER JOIN media_episodes AS episode ON episode.id = progress.media_episode_id
		INNER JOIN media_seasons AS season ON season.id = episode.season_id
		WHERE progress.user_id = ?
		ORDER BY progress.last_watched_at DESC
		LIMIT 1
	`

	item, err := scanWatchProgress(s.db.WithContext(ctx).Raw(query, userID).Row())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("find last watched: %w", err)
	}
	return &item, nil
}

func (s *PostgresHomeStore) findContinueWatching(ctx context.Context, userID string, limit int) ([]home.WatchProgressSummary, error) {
	const query = `
		SELECT
			episode.id::text,
			season.title,
			COALESCE(episode.cover_url, season.cover_url),
			progress.last_position_seconds,
			progress.duration_seconds
		FROM user_media_progress AS progress
		INNER JOIN media_episodes AS episode ON episode.id = progress.media_episode_id
		INNER JOIN media_seasons AS season ON season.id = episode.season_id
		WHERE progress.user_id = ? AND progress.completed = false
		ORDER BY progress.last_watched_at DESC
		LIMIT ?
	`

	rows, err := s.db.WithContext(ctx).Raw(query, userID, limit).Rows()
	if err != nil {
		return nil, fmt.Errorf("find continue watching: %w", err)
	}
	defer rows.Close()

	items := make([]home.WatchProgressSummary, 0, limit)
	for rows.Next() {
		item, err := scanWatchProgress(rows)
		if err != nil {
			return nil, fmt.Errorf("scan continue watching: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate continue watching: %w", err)
	}
	return items, nil
}

func scanWatchProgress(row rowScanner) (home.WatchProgressSummary, error) {
	var item home.WatchProgressSummary
	var coverURL sql.NullString
	if err := row.Scan(
		&item.MediaItemID,
		&item.Title,
		&coverURL,
		&item.LastPositionSeconds,
		&item.DurationSeconds,
	); err != nil {
		return home.WatchProgressSummary{}, err
	}
	if coverURL.Valid {
		item.CoverURL = &coverURL.String
	}
	return item, nil
}
