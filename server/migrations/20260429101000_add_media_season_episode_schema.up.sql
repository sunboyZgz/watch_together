CREATE TABLE media_seasons (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug TEXT NOT NULL UNIQUE CHECK (
        length(btrim(slug)) > 0 AND btrim(slug) = lower(btrim(slug))
    ),
    title TEXT NOT NULL CHECK (length(btrim(title)) > 0),
    original_title TEXT CHECK (
        original_title IS NULL OR length(btrim(original_title)) > 0
    ),
    description TEXT CHECK (
        description IS NULL OR length(btrim(description)) > 0
    ),
    cover_url TEXT CHECK (
        cover_url IS NULL OR length(btrim(cover_url)) > 0
    ),
    category TEXT CHECK (
        category IS NULL OR length(btrim(category)) > 0
    ),
    production_team TEXT CHECK (
        production_team IS NULL OR length(btrim(production_team)) > 0
    ),
    search_aliases JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (
        jsonb_typeof(search_aliases) = 'array'
    ),
    season_number INTEGER CHECK (season_number IS NULL OR season_number > 0),
    season_label TEXT CHECK (
        season_label IS NULL OR length(btrim(season_label)) > 0
    ),
    sort_order INTEGER NOT NULL DEFAULT 0 CHECK (sort_order >= 0),
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'active', 'archived')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE media_episodes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    season_id UUID NOT NULL REFERENCES media_seasons(id) ON DELETE CASCADE,
    legacy_media_item_id UUID UNIQUE REFERENCES media_items(id) ON DELETE SET NULL,
    title TEXT NOT NULL CHECK (length(btrim(title)) > 0),
    subtitle TEXT CHECK (
        subtitle IS NULL OR length(btrim(subtitle)) > 0
    ),
    description TEXT CHECK (
        description IS NULL OR length(btrim(description)) > 0
    ),
    cover_url TEXT CHECK (
        cover_url IS NULL OR length(btrim(cover_url)) > 0
    ),
    media_url TEXT NOT NULL CHECK (length(btrim(media_url)) > 0),
    duration_ms BIGINT CHECK (duration_ms IS NULL OR duration_ms > 0),
    episode_number INTEGER CHECK (episode_number IS NULL OR episode_number > 0),
    episode_label TEXT CHECK (
        episode_label IS NULL OR length(btrim(episode_label)) > 0
    ),
    source_key TEXT NOT NULL UNIQUE CHECK (length(btrim(source_key)) > 0),
    source_hash TEXT CHECK (
        source_hash IS NULL OR length(btrim(source_hash)) > 0
    ),
    sort_order INTEGER NOT NULL DEFAULT 0 CHECK (sort_order >= 0),
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'active', 'archived')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE media_season_tags (
    season_id UUID NOT NULL REFERENCES media_seasons(id) ON DELETE CASCADE,
    media_tag_id UUID NOT NULL REFERENCES media_tags(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (season_id, media_tag_id)
);

CREATE INDEX idx_media_seasons_status_sort
    ON media_seasons(status, sort_order);

CREATE INDEX idx_media_seasons_category
    ON media_seasons(category);

CREATE INDEX idx_media_seasons_original_title
    ON media_seasons(original_title);

CREATE INDEX idx_media_seasons_production_team
    ON media_seasons(production_team);

CREATE INDEX idx_media_seasons_search_aliases_gin
    ON media_seasons USING GIN(search_aliases);

CREATE INDEX idx_media_episodes_season_sort
    ON media_episodes(season_id, sort_order);

CREATE UNIQUE INDEX uniq_media_episodes_season_episode_number
    ON media_episodes(season_id, episode_number)
    WHERE episode_number IS NOT NULL;

CREATE INDEX idx_media_episodes_status
    ON media_episodes(status);

CREATE INDEX idx_media_episodes_source_hash
    ON media_episodes(source_hash)
    WHERE source_hash IS NOT NULL;

CREATE INDEX idx_media_season_tags_media_tag_id
    ON media_season_tags(media_tag_id);

WITH legacy_seasons AS (
    INSERT INTO media_seasons (
        slug,
        title,
        original_title,
        description,
        cover_url,
        category,
        production_team,
        search_aliases,
        season_label,
        status,
        created_at,
        updated_at
    )
    SELECT
        'legacy-media-item-' || replace(item.id::text, '-', ''),
        item.title,
        NULLIF(btrim(item.original_title), ''),
        NULLIF(btrim(item.description), ''),
        NULLIF(btrim(item.cover_url), ''),
        NULLIF(btrim(item.category), ''),
        NULLIF(btrim(item.production_team), ''),
        item.search_aliases,
        NULLIF(btrim(item.season_label), ''),
        item.status,
        item.created_at,
        item.updated_at
    FROM media_items AS item
    RETURNING id, slug
)
INSERT INTO media_episodes (
    season_id,
    legacy_media_item_id,
    title,
    subtitle,
    description,
    cover_url,
    media_url,
    duration_ms,
    episode_label,
    source_key,
    status,
    created_at,
    updated_at
)
SELECT
    season.id,
    item.id,
    item.title,
    NULLIF(btrim(item.subtitle), ''),
    NULLIF(btrim(item.description), ''),
    NULLIF(btrim(item.cover_url), ''),
    item.media_url,
    CASE WHEN item.duration_ms > 0 THEN item.duration_ms ELSE NULL END,
    NULLIF(btrim(item.episode_label), ''),
    'legacy/media_items/' || item.id::text || '/source',
    item.status,
    item.created_at,
    item.updated_at
FROM media_items AS item
INNER JOIN legacy_seasons AS season
    ON season.slug = 'legacy-media-item-' || replace(item.id::text, '-', '');

INSERT INTO media_season_tags (season_id, media_tag_id, created_at)
SELECT DISTINCT
    episode.season_id,
    item_tag.media_tag_id,
    item_tag.created_at
FROM media_item_tags AS item_tag
INNER JOIN media_episodes AS episode
    ON episode.legacy_media_item_id = item_tag.media_item_id
ON CONFLICT DO NOTHING;
