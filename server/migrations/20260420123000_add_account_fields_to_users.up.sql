ALTER TABLE users
    ADD COLUMN account TEXT,
    ADD COLUMN password_hash TEXT;

UPDATE users
SET
    account = CONCAT('user_', replace(id::text, '-', '')),
    password_hash = 'PENDING_PASSWORD_HASH'
WHERE account IS NULL OR password_hash IS NULL;

ALTER TABLE users
    ALTER COLUMN account SET NOT NULL,
    ALTER COLUMN password_hash SET NOT NULL;

ALTER TABLE users
    ADD CONSTRAINT users_account_not_blank CHECK (length(btrim(account)) > 0),
    ADD CONSTRAINT users_password_hash_not_blank CHECK (length(btrim(password_hash)) > 0);

CREATE UNIQUE INDEX users_account_key ON users(account);
