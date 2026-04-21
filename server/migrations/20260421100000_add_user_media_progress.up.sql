CREATE TABLE user_media_progress (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    media_item_id UUID NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    last_position_seconds INTEGER NOT NULL CHECK (last_position_seconds >= 0),
    duration_seconds INTEGER NOT NULL CHECK (duration_seconds > 0),
    last_watched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed BOOLEAN NOT NULL DEFAULT FALSE,
    completion_source TEXT CHECK (
        completion_source IS NULL OR
        completion_source IN ('ended', 'manual_mark', 'threshold_auto')
    ),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT user_media_progress_position_within_duration
        CHECK (last_position_seconds <= duration_seconds)
);

CREATE UNIQUE INDEX uniq_user_media_progress_user_media
    ON user_media_progress(user_id, media_item_id);

CREATE INDEX idx_user_media_progress_user_last_watched
    ON user_media_progress(user_id, last_watched_at DESC);

CREATE INDEX idx_user_media_progress_user_completed_last_watched
    ON user_media_progress(user_id, completed, last_watched_at DESC);
