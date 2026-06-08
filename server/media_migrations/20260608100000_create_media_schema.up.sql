CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE media_tags (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL CHECK (length(btrim(name)) > 0),
    slug TEXT NOT NULL UNIQUE CHECK (
        length(btrim(slug)) > 0 AND btrim(slug) = lower(btrim(slug))
    ),
    sort_order INTEGER NOT NULL DEFAULT 0 CHECK (sort_order >= 0),
    is_featured BOOLEAN NOT NULL DEFAULT FALSE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

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

CREATE TABLE media_episode_variants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    media_episode_id UUID NOT NULL REFERENCES media_episodes(id) ON DELETE CASCADE,
    variant_key TEXT NOT NULL CHECK (length(btrim(variant_key)) > 0),
    label TEXT NOT NULL CHECK (length(btrim(label)) > 0),
    playlist_url TEXT NOT NULL CHECK (length(btrim(playlist_url)) > 0),
    width INTEGER CHECK (width IS NULL OR width > 0),
    height INTEGER CHECK (height IS NULL OR height > 0),
    bandwidth_bps INTEGER CHECK (bandwidth_bps IS NULL OR bandwidth_bps > 0),
    codecs TEXT CHECK (codecs IS NULL OR length(btrim(codecs)) > 0),
    is_default BOOLEAN NOT NULL DEFAULT false,
    sort_order INTEGER NOT NULL DEFAULT 0 CHECK (sort_order >= 0),
    segment_count INTEGER CHECK (segment_count IS NULL OR segment_count > 0),
    average_segment_ms INTEGER CHECK (average_segment_ms IS NULL OR average_segment_ms > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (media_episode_id, variant_key)
);

CREATE INDEX idx_media_tags_active_sort
    ON media_tags(is_active, sort_order);

CREATE INDEX idx_media_tags_featured_active_sort
    ON media_tags(is_featured, is_active, sort_order);

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

CREATE INDEX idx_media_episode_variants_episode_sort
    ON media_episode_variants(media_episode_id, sort_order);

CREATE INDEX idx_media_episode_variants_height
    ON media_episode_variants(height)
    WHERE height IS NOT NULL;
