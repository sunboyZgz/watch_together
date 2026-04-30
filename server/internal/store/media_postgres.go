package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"watch_together/server/internal/media"
)

type PostgresMediaStore struct {
	db *sql.DB
}

// NewPostgresMediaStore creates the PostgreSQL-backed repository for media catalog data.
func NewPostgresMediaStore(db *sql.DB) *PostgresMediaStore {
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
		LIMIT $1
	`

	rows, err := s.db.QueryContext(ctx, query, limit)
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
				$1 = ''
				OR season.title ILIKE '%' || $1 || '%'
				OR season.description ILIKE '%' || $1 || '%'
				OR season.original_title ILIKE '%' || $1 || '%'
				OR season.production_team ILIKE '%' || $1 || '%'
				OR season.search_aliases::text ILIKE '%' || $1 || '%'
				OR episode.title ILIKE '%' || $1 || '%'
				OR episode.subtitle ILIKE '%' || $1 || '%'
				OR episode.description ILIKE '%' || $1 || '%'
				OR episode.episode_label ILIKE '%' || $1 || '%'
			)
			AND (
				$2 = ''
				OR EXISTS (
					SELECT 1
					FROM media_season_tags AS filter_season_tag
					INNER JOIN media_tags AS filter_tag ON filter_tag.id = filter_season_tag.media_tag_id
					WHERE filter_season_tag.season_id = season.id
						AND filter_tag.slug = $2
						AND filter_tag.is_active = true
				)
			)
		GROUP BY episode.id, season.id
		ORDER BY season.sort_order ASC, episode.sort_order ASC, season.title ASC, episode.episode_number ASC NULLS LAST
		LIMIT $3 OFFSET $4
	`

	rows, err := s.db.QueryContext(ctx, query, params.Query, params.Tag, params.Limit, params.Offset)
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
