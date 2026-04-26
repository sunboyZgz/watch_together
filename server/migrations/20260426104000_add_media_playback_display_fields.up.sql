ALTER TABLE media_items
    ADD COLUMN season_label TEXT,
    ADD COLUMN episode_label TEXT;

ALTER TABLE media_items
    ADD CONSTRAINT media_items_season_label_not_blank CHECK (
        season_label IS NULL OR length(btrim(season_label)) > 0
    ),
    ADD CONSTRAINT media_items_episode_label_not_blank CHECK (
        episode_label IS NULL OR length(btrim(episode_label)) > 0
    );

CREATE INDEX idx_media_items_season_label ON media_items(season_label);
CREATE INDEX idx_media_items_episode_label ON media_items(episode_label);
