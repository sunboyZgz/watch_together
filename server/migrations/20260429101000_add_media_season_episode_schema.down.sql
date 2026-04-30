DROP INDEX IF EXISTS idx_media_season_tags_media_tag_id;
DROP INDEX IF EXISTS idx_media_episodes_source_hash;
DROP INDEX IF EXISTS idx_media_episodes_status;
DROP INDEX IF EXISTS uniq_media_episodes_season_episode_number;
DROP INDEX IF EXISTS idx_media_episodes_season_sort;
DROP INDEX IF EXISTS idx_media_seasons_search_aliases_gin;
DROP INDEX IF EXISTS idx_media_seasons_production_team;
DROP INDEX IF EXISTS idx_media_seasons_original_title;
DROP INDEX IF EXISTS idx_media_seasons_category;
DROP INDEX IF EXISTS idx_media_seasons_status_sort;

DROP TABLE IF EXISTS media_season_tags;
DROP TABLE IF EXISTS media_episodes;
DROP TABLE IF EXISTS media_seasons;
