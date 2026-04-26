DROP INDEX IF EXISTS idx_media_items_episode_label;
DROP INDEX IF EXISTS idx_media_items_season_label;

ALTER TABLE media_items
    DROP CONSTRAINT IF EXISTS media_items_episode_label_not_blank,
    DROP CONSTRAINT IF EXISTS media_items_season_label_not_blank,
    DROP COLUMN IF EXISTS episode_label,
    DROP COLUMN IF EXISTS season_label;
