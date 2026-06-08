ALTER TABLE rooms
    DROP CONSTRAINT IF EXISTS rooms_media_episode_id_fkey;

ALTER TABLE user_media_progress
    DROP CONSTRAINT IF EXISTS user_media_progress_media_episode_id_fkey;
