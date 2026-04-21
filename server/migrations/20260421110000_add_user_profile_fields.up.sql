ALTER TABLE users
    ADD COLUMN avatar_seed TEXT,
    ADD COLUMN avatar_url TEXT,
    ADD COLUMN bio TEXT;

UPDATE users
SET avatar_seed = CONCAT('avatar_', replace(id::text, '-', ''))
WHERE avatar_seed IS NULL;

ALTER TABLE users
    ALTER COLUMN avatar_seed SET NOT NULL;

ALTER TABLE users
    ADD CONSTRAINT users_avatar_seed_not_blank CHECK (length(btrim(avatar_seed)) > 0),
    ADD CONSTRAINT users_avatar_url_not_blank CHECK (
        avatar_url IS NULL OR length(btrim(avatar_url)) > 0
    ),
    ADD CONSTRAINT users_bio_not_blank CHECK (
        bio IS NULL OR length(btrim(bio)) > 0
    );
