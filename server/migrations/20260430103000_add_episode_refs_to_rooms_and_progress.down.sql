DROP INDEX IF EXISTS idx_user_media_progress_episode_id;
DROP INDEX IF EXISTS uniq_user_media_progress_user_episode;

ALTER TABLE user_media_progress
    DROP CONSTRAINT IF EXISTS user_media_progress_has_media_reference;

UPDATE user_media_progress
SET media_item_id = episode.legacy_media_item_id
FROM media_episodes AS episode
WHERE user_media_progress.media_episode_id = episode.id
    AND user_media_progress.media_item_id IS NULL
    AND episode.legacy_media_item_id IS NOT NULL;

ALTER TABLE user_media_progress
    ALTER COLUMN media_item_id SET NOT NULL,
    DROP COLUMN IF EXISTS media_episode_id;

DROP INDEX IF EXISTS idx_rooms_media_episode_id;

ALTER TABLE rooms
    DROP CONSTRAINT IF EXISTS rooms_has_media_reference;

UPDATE rooms
SET media_item_id = episode.legacy_media_item_id
FROM media_episodes AS episode
WHERE rooms.media_episode_id = episode.id
    AND rooms.media_item_id IS NULL
    AND episode.legacy_media_item_id IS NOT NULL;

ALTER TABLE rooms
    ALTER COLUMN media_item_id SET NOT NULL,
    DROP COLUMN IF EXISTS media_episode_id;
