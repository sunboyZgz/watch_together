CREATE TABLE IF NOT EXISTS room_timeline_outbox (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    topic TEXT NOT NULL,
    event_id TEXT NOT NULL UNIQUE,
    event_type TEXT NOT NULL,
    room_id TEXT NOT NULL,
    payload JSONB NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT room_timeline_outbox_status_check
        CHECK (status IN ('pending', 'publishing', 'published'))
);

CREATE INDEX IF NOT EXISTS idx_room_timeline_outbox_pending
    ON room_timeline_outbox (status, next_attempt_at, created_at)
    WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS idx_room_timeline_outbox_room
    ON room_timeline_outbox (room_id, created_at);
