CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE rooms (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    room_code TEXT NOT NULL UNIQUE CHECK (length(room_code) = 6),
    host_user_id UUID NOT NULL,
    media_episode_id UUID NOT NULL,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'grace_period', 'destroyed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_empty_at TIMESTAMPTZ,
    destroy_after TIMESTAMPTZ
);

CREATE TABLE room_members (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('host', 'member')),
    joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    left_at TIMESTAMPTZ,
    is_active BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE INDEX idx_rooms_host_user_id ON rooms(host_user_id);
CREATE INDEX idx_rooms_media_episode_id ON rooms(media_episode_id);
CREATE INDEX idx_rooms_status ON rooms(status);
CREATE INDEX idx_rooms_destroy_after ON rooms(destroy_after);

CREATE INDEX idx_room_members_room_id ON room_members(room_id);
CREATE INDEX idx_room_members_user_id ON room_members(user_id);
CREATE INDEX idx_room_members_active_room_id ON room_members(room_id) WHERE is_active = TRUE;
CREATE UNIQUE INDEX uniq_room_members_active_user_per_room
    ON room_members(room_id, user_id)
    WHERE is_active = TRUE;
