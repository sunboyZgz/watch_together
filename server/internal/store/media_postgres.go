package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"watch_together/server/internal/media"
)

type PostgresMediaStore struct {
	db *gorm.DB
}

// FindPlaybackItem resolves the raw HLS playback URL for a signed playback entrypoint.
func (s *PostgresMediaStore) FindPlaybackItem(ctx context.Context, episodeID string) (media.PlaybackItem, error) {
	const query = `
		SELECT episode.id::text, episode.media_url
		FROM media_episodes AS episode
		INNER JOIN media_seasons AS season ON season.id = episode.season_id
		WHERE episode.id = ?
			AND episode.status = 'active'
			AND season.status = 'active'
		LIMIT 1
	`
	var item media.PlaybackItem
	if err := s.db.WithContext(ctx).Raw(query, episodeID).Row().Scan(&item.ID, &item.MediaURL); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return media.PlaybackItem{}, media.ErrMediaNotFound
		}
		return media.PlaybackItem{}, fmt.Errorf("find media playback item: %w", err)
	}
	return item, nil
}

// FindEpisodeDetail loads the active episode metadata used by room bootstrap flows.
func (s *PostgresMediaStore) FindEpisodeDetail(ctx context.Context, episodeID string) (media.EpisodeDetail, error) {
	const query = `
		SELECT
			episode.id::text,
			season.title,
			episode.subtitle,
			episode.media_url,
			episode.duration_ms,
			season.season_label,
			episode.episode_label
		FROM media_episodes AS episode
		INNER JOIN media_seasons AS season ON season.id = episode.season_id
		WHERE episode.id = ?
			AND episode.status = 'active'
			AND season.status = 'active'
		LIMIT 1
	`
	var detail media.EpisodeDetail
	var subtitle sql.NullString
	var durationMs sql.NullInt64
	var seasonLabel sql.NullString
	var episodeLabel sql.NullString
	if err := s.db.WithContext(ctx).Raw(query, episodeID).Row().Scan(
		&detail.ID,
		&detail.Title,
		&subtitle,
		&detail.MediaURL,
		&durationMs,
		&seasonLabel,
		&episodeLabel,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return media.EpisodeDetail{}, media.ErrMediaNotFound
		}
		return media.EpisodeDetail{}, fmt.Errorf("find media episode detail: %w", err)
	}
	detail.Subtitle = nullableStringPtr(subtitle)
	if durationMs.Valid {
		detail.DurationMs = &durationMs.Int64
	}
	detail.SeasonLabel = nullableStringPtr(seasonLabel)
	detail.EpisodeLabel = nullableStringPtr(episodeLabel)
	return detail, nil
}

// ValidatePlayableEpisode returns the canonical active episode id for write-side callers.
func (s *PostgresMediaStore) ValidatePlayableEpisode(ctx context.Context, episodeID string) (media.PlayableEpisode, error) {
	const query = `
		SELECT episode.id::text
		FROM media_episodes AS episode
		INNER JOIN media_seasons AS season ON season.id = episode.season_id
		WHERE episode.id = ?
			AND episode.status = 'active'
			AND season.status = 'active'
		LIMIT 1
	`
	var canonicalID string
	if err := s.db.WithContext(ctx).Raw(query, episodeID).Row().Scan(&canonicalID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return media.PlayableEpisode{}, media.ErrMediaNotFound
		}
		return media.PlayableEpisode{}, fmt.Errorf("validate playable episode: %w", err)
	}
	return media.PlayableEpisode{ID: canonicalID, Playable: true}, nil
}

func (s *PostgresMediaStore) BatchFindEpisodeSummaries(ctx context.Context, episodeIDs []string) ([]media.EpisodeSummary, error) {
	if len(episodeIDs) == 0 {
		return nil, nil
	}
	const query = `
		SELECT
			episode.id::text,
			season.title,
			COALESCE(episode.cover_url, season.cover_url) AS cover_url
		FROM media_episodes AS episode
		INNER JOIN media_seasons AS season ON season.id = episode.season_id
		WHERE episode.id IN ?
	`
	rows, err := s.db.WithContext(ctx).Raw(query, episodeIDs).Rows()
	if err != nil {
		return nil, fmt.Errorf("batch find episode summaries: %w", err)
	}
	defer rows.Close()

	summaries := make([]media.EpisodeSummary, 0, len(episodeIDs))
	for rows.Next() {
		var summary media.EpisodeSummary
		var coverURL sql.NullString
		if err := rows.Scan(&summary.ID, &summary.Title, &coverURL); err != nil {
			return nil, fmt.Errorf("scan episode summary: %w", err)
		}
		summary.CoverURL = nullableStringPtr(coverURL)
		summaries = append(summaries, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate episode summaries: %w", err)
	}
	return summaries, nil
}

// NewPostgresMediaStore creates the PostgreSQL-backed repository for media catalog data.
func NewPostgresMediaStore(db *gorm.DB) *PostgresMediaStore {
	return &PostgresMediaStore{db: db}
}

// ListTags returns active featured tags and the active expanded tag list.
func (s *PostgresMediaStore) ListTags(ctx context.Context, allLimit int) (media.TagList, error) {
	featuredTags, err := s.listTags(ctx, true, 5)
	if err != nil {
		return media.TagList{}, err
	}
	allTags, err := s.listTags(ctx, false, allLimit)
	if err != nil {
		return media.TagList{}, err
	}
	return media.TagList{
		FeaturedTags: featuredTags,
		AllTags:      allTags,
	}, nil
}

func (s *PostgresMediaStore) listTags(ctx context.Context, featuredOnly bool, limit int) ([]media.Tag, error) {
	query := `
		SELECT id::text, slug, name
		FROM media_tags
		WHERE is_active = true
	`
	if featuredOnly {
		query += ` AND is_featured = true`
	}
	query += `
		ORDER BY sort_order ASC, name ASC
		LIMIT ?
	`

	rows, err := s.db.WithContext(ctx).Raw(query, limit).Rows()
	if err != nil {
		return nil, fmt.Errorf("list media tags: %w", err)
	}
	defer rows.Close()

	tags := make([]media.Tag, 0, limit)
	for rows.Next() {
		var tag media.Tag
		if err := rows.Scan(&tag.ID, &tag.Slug, &tag.Name); err != nil {
			return nil, fmt.Errorf("scan media tag: %w", err)
		}
		tags = append(tags, tag)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate media tags: %w", err)
	}
	return tags, nil
}

// SearchItems applies optional text search and tag filtering to active episode-backed media.
func (s *PostgresMediaStore) SearchItems(ctx context.Context, params media.StoreSearchParams) ([]media.Item, error) {
	const query = `
		SELECT
			episode.id::text,
			season.title,
            episode.subtitle,
            COALESCE(episode.description, season.description) AS description,
            COALESCE(episode.cover_url, season.cover_url) AS cover_url,
            episode.media_url,
            episode.duration_ms,
			season.season_label,
			episode.episode_label,
			COALESCE(
				jsonb_agg(
					DISTINCT jsonb_build_object('slug', tag.slug, 'name', tag.name)
				) FILTER (WHERE tag.id IS NOT NULL),
				'[]'::jsonb
			)::text AS tags
		FROM media_episodes AS episode
		INNER JOIN media_seasons AS season ON season.id = episode.season_id
		LEFT JOIN media_season_tags AS season_tag ON season_tag.season_id = season.id
		LEFT JOIN media_tags AS tag ON tag.id = season_tag.media_tag_id AND tag.is_active = true
		WHERE episode.status = 'active'
			AND season.status = 'active'
			AND (
				? = ''
				OR season.title ILIKE '%' || ? || '%'
				OR season.description ILIKE '%' || ? || '%'
				OR season.original_title ILIKE '%' || ? || '%'
				OR season.production_team ILIKE '%' || ? || '%'
				OR season.search_aliases::text ILIKE '%' || ? || '%'
				OR episode.title ILIKE '%' || ? || '%'
				OR episode.subtitle ILIKE '%' || ? || '%'
				OR episode.description ILIKE '%' || ? || '%'
				OR episode.episode_label ILIKE '%' || ? || '%'
			)
			AND (
				? = ''
				OR EXISTS (
					SELECT 1
					FROM media_season_tags AS filter_season_tag
					INNER JOIN media_tags AS filter_tag ON filter_tag.id = filter_season_tag.media_tag_id
					WHERE filter_season_tag.season_id = season.id
						AND filter_tag.slug = ?
						AND filter_tag.is_active = true
				)
			)
		GROUP BY episode.id, season.id
		ORDER BY season.sort_order ASC, episode.sort_order ASC, season.title ASC, episode.episode_number ASC NULLS LAST
		LIMIT ? OFFSET ?
	`

	queryArg := params.Query
	rows, err := s.db.WithContext(ctx).Raw(
		query,
		queryArg,
		queryArg,
		queryArg,
		queryArg,
		queryArg,
		queryArg,
		queryArg,
		queryArg,
		queryArg,
		queryArg,
		params.Tag,
		params.Tag,
		params.Limit,
		params.Offset,
	).Rows()
	if err != nil {
		return nil, fmt.Errorf("search media items: %w", err)
	}
	defer rows.Close()

	items := make([]media.Item, 0, params.Limit)
	for rows.Next() {
		item, err := scanMediaItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate media items: %w", err)
	}
	return items, nil
}

func scanMediaItem(row rowScanner) (media.Item, error) {
	var item media.Item
	var subtitle sql.NullString
	var description sql.NullString
	var coverURL sql.NullString
	var mediaURL sql.NullString
	var durationMs sql.NullInt64
	var seasonLabel sql.NullString
	var episodeLabel sql.NullString
	var tagsJSON string

	if err := row.Scan(
		&item.ID,
		&item.Title,
		&subtitle,
		&description,
		&coverURL,
		&mediaURL,
		&durationMs,
		&seasonLabel,
		&episodeLabel,
		&tagsJSON,
	); err != nil {
		return media.Item{}, fmt.Errorf("scan media item: %w", err)
	}

	item.Subtitle = nullableStringPtr(subtitle)
	item.Description = nullableStringPtr(description)
	item.CoverURL = nullableStringPtr(coverURL)
	item.MediaURL = mediaURL.String
	if durationMs.Valid {
		item.DurationMs = &durationMs.Int64
	}
	item.SeasonLabel = nullableStringPtr(seasonLabel)
	item.EpisodeLabel = nullableStringPtr(episodeLabel)
	if err := json.Unmarshal([]byte(tagsJSON), &item.Tags); err != nil {
		return media.Item{}, fmt.Errorf("decode media item tags: %w", err)
	}
	return item, nil
}

func nullableStringPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}
