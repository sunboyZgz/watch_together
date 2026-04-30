ALTER TABLE rooms
    ADD COLUMN media_episode_id UUID REFERENCES media_episodes(id);

UPDATE rooms
SET media_episode_id = episode.id
FROM media_episodes AS episode
WHERE episode.legacy_media_item_id = rooms.media_item_id
    AND rooms.media_episode_id IS NULL;

ALTER TABLE rooms
    ALTER COLUMN media_item_id DROP NOT NULL,
    ADD CONSTRAINT rooms_has_media_reference CHECK (
        media_item_id IS NOT NULL OR media_episode_id IS NOT NULL
    );

CREATE INDEX idx_rooms_media_episode_id
    ON rooms(media_episode_id);

ALTER TABLE user_media_progress
    ADD COLUMN media_episode_id UUID REFERENCES media_episodes(id) ON DELETE CASCADE;

UPDATE user_media_progress
SET media_episode_id = episode.id
FROM media_episodes AS episode
WHERE episode.legacy_media_item_id = user_media_progress.media_item_id
    AND user_media_progress.media_episode_id IS NULL;

ALTER TABLE user_media_progress
    ALTER COLUMN media_item_id DROP NOT NULL,
    ADD CONSTRAINT user_media_progress_has_media_reference CHECK (
        media_item_id IS NOT NULL OR media_episode_id IS NOT NULL
    );

CREATE UNIQUE INDEX uniq_user_media_progress_user_episode
    ON user_media_progress(user_id, media_episode_id)
    WHERE media_episode_id IS NOT NULL;

CREATE INDEX idx_user_media_progress_episode_id
    ON user_media_progress(media_episode_id);
