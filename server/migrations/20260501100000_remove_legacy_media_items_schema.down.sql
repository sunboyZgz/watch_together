CREATE TABLE media_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title TEXT NOT NULL CHECK (length(btrim(title)) > 0),
    subtitle TEXT,
    description TEXT,
    cover_url TEXT,
    media_url TEXT NOT NULL CHECK (length(btrim(media_url)) > 0),
    category TEXT,
    tags JSONB NOT NULL DEFAULT '[]'::jsonb,
    duration_ms BIGINT,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('draft', 'active', 'archived')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    original_title TEXT,
    production_team TEXT,
    search_aliases JSONB NOT NULL DEFAULT '[]'::jsonb,
    season_label TEXT,
    episode_label TEXT,
    CONSTRAINT media_items_original_title_not_blank CHECK (
        original_title IS NULL OR length(btrim(original_title)) > 0
    ),
    CONSTRAINT media_items_production_team_not_blank CHECK (
        production_team IS NULL OR length(btrim(production_team)) > 0
    ),
    CONSTRAINT media_items_search_aliases_is_array CHECK (
        jsonb_typeof(search_aliases) = 'array'
    ),
    CONSTRAINT media_items_season_label_not_blank CHECK (
        season_label IS NULL OR length(btrim(season_label)) > 0
    ),
    CONSTRAINT media_items_episode_label_not_blank CHECK (
        episode_label IS NULL OR length(btrim(episode_label)) > 0
    )
);

CREATE INDEX idx_media_items_category ON media_items(category);
CREATE INDEX idx_media_items_status ON media_items(status);
CREATE INDEX idx_media_items_tags_gin ON media_items USING GIN(tags);
CREATE INDEX idx_media_items_original_title ON media_items(original_title);
CREATE INDEX idx_media_items_production_team ON media_items(production_team);
CREATE INDEX idx_media_items_search_aliases_gin ON media_items USING GIN(search_aliases);
CREATE INDEX idx_media_items_season_label ON media_items(season_label);
CREATE INDEX idx_media_items_episode_label ON media_items(episode_label);

CREATE TABLE media_item_tags (
    media_item_id UUID NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    media_tag_id UUID NOT NULL REFERENCES media_tags(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (media_item_id, media_tag_id)
);

CREATE INDEX idx_media_item_tags_media_tag_id
    ON media_item_tags(media_tag_id);

ALTER TABLE rooms
    ADD COLUMN media_item_id UUID REFERENCES media_items(id),
    ALTER COLUMN media_episode_id DROP NOT NULL,
    ADD CONSTRAINT rooms_has_media_reference CHECK (
        media_item_id IS NOT NULL OR media_episode_id IS NOT NULL
    );

CREATE INDEX idx_rooms_media_item_id
    ON rooms(media_item_id);

ALTER TABLE user_media_progress
    ADD COLUMN media_item_id UUID REFERENCES media_items(id) ON DELETE CASCADE,
    ALTER COLUMN media_episode_id DROP NOT NULL,
    ADD CONSTRAINT user_media_progress_has_media_reference CHECK (
        media_item_id IS NOT NULL OR media_episode_id IS NOT NULL
    );

DROP INDEX IF EXISTS uniq_user_media_progress_user_episode;

CREATE UNIQUE INDEX uniq_user_media_progress_user_episode
    ON user_media_progress(user_id, media_episode_id)
    WHERE media_episode_id IS NOT NULL;

CREATE UNIQUE INDEX uniq_user_media_progress_user_media
    ON user_media_progress(user_id, media_item_id)
    WHERE media_item_id IS NOT NULL;

ALTER TABLE media_episodes
    ADD COLUMN legacy_media_item_id UUID UNIQUE REFERENCES media_items(id) ON DELETE SET NULL;
