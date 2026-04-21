ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_bio_not_blank,
    DROP CONSTRAINT IF EXISTS users_avatar_url_not_blank,
    DROP CONSTRAINT IF EXISTS users_avatar_seed_not_blank;

ALTER TABLE users
    DROP COLUMN IF EXISTS bio,
    DROP COLUMN IF EXISTS avatar_url,
    DROP COLUMN IF EXISTS avatar_seed;
