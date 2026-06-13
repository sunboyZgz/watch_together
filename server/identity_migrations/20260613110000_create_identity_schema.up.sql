CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    nickname TEXT NOT NULL,
    avatar_seed TEXT NOT NULL,
    avatar_url TEXT,
    bio TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT users_account_not_blank CHECK (length(btrim(account)) > 0),
    CONSTRAINT users_password_hash_not_blank CHECK (length(btrim(password_hash)) > 0),
    CONSTRAINT users_nickname_not_blank CHECK (length(btrim(nickname)) > 0),
    CONSTRAINT users_avatar_seed_not_blank CHECK (length(btrim(avatar_seed)) > 0),
    CONSTRAINT users_avatar_url_not_blank CHECK (
        avatar_url IS NULL OR length(btrim(avatar_url)) > 0
    ),
    CONSTRAINT users_bio_not_blank CHECK (
        bio IS NULL OR length(btrim(bio)) > 0
    )
);

CREATE UNIQUE INDEX users_account_key ON users(account);
