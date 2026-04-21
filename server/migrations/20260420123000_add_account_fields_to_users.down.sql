DROP INDEX IF EXISTS users_account_key;

ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_password_hash_not_blank,
    DROP CONSTRAINT IF EXISTS users_account_not_blank;

ALTER TABLE users
    DROP COLUMN IF EXISTS password_hash,
    DROP COLUMN IF EXISTS account;
