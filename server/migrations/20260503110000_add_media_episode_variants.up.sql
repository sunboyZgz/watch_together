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
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (media_episode_id, variant_key)
);

CREATE INDEX idx_media_episode_variants_episode_sort
    ON media_episode_variants(media_episode_id, sort_order);

CREATE INDEX idx_media_episode_variants_height
    ON media_episode_variants(height)
    WHERE height IS NOT NULL;
