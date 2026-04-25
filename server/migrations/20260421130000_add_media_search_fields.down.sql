DROP INDEX IF EXISTS idx_media_items_search_aliases_gin;
DROP INDEX IF EXISTS idx_media_items_production_team;
DROP INDEX IF EXISTS idx_media_items_original_title;

ALTER TABLE media_items
    DROP CONSTRAINT IF EXISTS media_items_search_aliases_is_array,
    DROP CONSTRAINT IF EXISTS media_items_production_team_not_blank,
    DROP CONSTRAINT IF EXISTS media_items_original_title_not_blank,
    DROP COLUMN IF EXISTS search_aliases,
    DROP COLUMN IF EXISTS production_team,
    DROP COLUMN IF EXISTS original_title;
