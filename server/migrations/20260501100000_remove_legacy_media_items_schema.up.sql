DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM rooms WHERE media_episode_id IS NULL) THEN
        RAISE EXCEPTION 'cannot remove rooms.media_item_id while rooms.media_episode_id has null rows';
    END IF;

    IF EXISTS (SELECT 1 FROM user_media_progress WHERE media_episode_id IS NULL) THEN
        RAISE EXCEPTION 'cannot remove user_media_progress.media_item_id while media_episode_id has null rows';
    END IF;
END $$;

ALTER TABLE rooms
    DROP CONSTRAINT IF EXISTS rooms_has_media_reference,
    DROP CONSTRAINT IF EXISTS rooms_media_item_id_fkey,
    ALTER COLUMN media_episode_id SET NOT NULL,
    DROP COLUMN IF EXISTS media_item_id;

DROP INDEX IF EXISTS idx_rooms_media_item_id;

ALTER TABLE user_media_progress
    DROP CONSTRAINT IF EXISTS user_media_progress_has_media_reference,
    DROP CONSTRAINT IF EXISTS user_media_progress_media_item_id_fkey,
    ALTER COLUMN media_episode_id SET NOT NULL,
    DROP COLUMN IF EXISTS media_item_id;

DROP INDEX IF EXISTS uniq_user_media_progress_user_media;
DROP INDEX IF EXISTS uniq_user_media_progress_user_episode;

CREATE UNIQUE INDEX uniq_user_media_progress_user_episode
    ON user_media_progress(user_id, media_episode_id);

ALTER TABLE media_episodes
    DROP CONSTRAINT IF EXISTS media_episodes_legacy_media_item_id_fkey,
    DROP CONSTRAINT IF EXISTS media_episodes_legacy_media_item_id_key,
    DROP COLUMN IF EXISTS legacy_media_item_id;

DROP TABLE IF EXISTS media_item_tags;
DROP TABLE IF EXISTS media_items;
