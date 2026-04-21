CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    nickname TEXT NOT NULL CHECK (length(btrim(nickname)) > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE media_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title TEXT NOT NULL CHECK (length(btrim(title)) > 0),
    subtitle TEXT,
    description TEXT,
    cover_url TEXT,
    media_url TEXT NOT NULL CHECK (length(btrim(media_url)) > 0),
    category TEXT,
    tags JSONB NOT NULL DEFAULT '[]'::jsonb,
    duration_ms BIGINT,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('draft', 'active', 'archived')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE rooms (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    room_code TEXT NOT NULL UNIQUE CHECK (length(room_code) = 6),
    host_user_id UUID NOT NULL REFERENCES users(id),
    media_item_id UUID NOT NULL REFERENCES media_items(id),
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'grace_period', 'destroyed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_empty_at TIMESTAMPTZ,
    destroy_after TIMESTAMPTZ
);

CREATE TABLE room_members (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id),
    role TEXT NOT NULL CHECK (role IN ('host', 'member')),
    joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    left_at TIMESTAMPTZ,
    is_active BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE INDEX idx_media_items_category ON media_items(category);
CREATE INDEX idx_media_items_status ON media_items(status);
CREATE INDEX idx_media_items_tags_gin ON media_items USING GIN(tags);

CREATE INDEX idx_rooms_host_user_id ON rooms(host_user_id);
CREATE INDEX idx_rooms_media_item_id ON rooms(media_item_id);
CREATE INDEX idx_rooms_status ON rooms(status);
CREATE INDEX idx_rooms_destroy_after ON rooms(destroy_after);

CREATE INDEX idx_room_members_room_id ON room_members(room_id);
CREATE INDEX idx_room_members_user_id ON room_members(user_id);
CREATE INDEX idx_room_members_active_room_id ON room_members(room_id) WHERE is_active = TRUE;
CREATE UNIQUE INDEX uniq_room_members_active_user_per_room
    ON room_members(room_id, user_id)
    WHERE is_active = TRUE;
