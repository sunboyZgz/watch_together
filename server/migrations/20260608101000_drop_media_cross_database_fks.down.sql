DO $$
BEGIN
    IF to_regclass('public.media_episodes') IS NOT NULL THEN
        IF NOT EXISTS (
            SELECT 1
            FROM pg_constraint
            WHERE conname = 'rooms_media_episode_id_fkey'
        ) THEN
            ALTER TABLE rooms
                ADD CONSTRAINT rooms_media_episode_id_fkey
                FOREIGN KEY (media_episode_id) REFERENCES media_episodes(id);
        END IF;

        IF NOT EXISTS (
            SELECT 1
            FROM pg_constraint
            WHERE conname = 'user_media_progress_media_episode_id_fkey'
        ) THEN
            ALTER TABLE user_media_progress
                ADD CONSTRAINT user_media_progress_media_episode_id_fkey
                FOREIGN KEY (media_episode_id) REFERENCES media_episodes(id) ON DELETE CASCADE;
        END IF;
    END IF;
END $$;
