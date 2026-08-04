-- +migrate Up

ALTER TABLE users ADD COLUMN login TEXT;
ALTER TABLE users ADD COLUMN display_name TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN email_verified INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN google_sub TEXT;
ALTER TABLE users ADD COLUMN disabled INTEGER NOT NULL DEFAULT 0;

-- Bootstrap existing rows (admin created before this migration).
UPDATE users SET login = lower(substr(email, 1, instr(email || '@', '@') - 1))
 WHERE login IS NULL OR login = '';
UPDATE users SET email_verified = 1 WHERE email_verified = 0;
UPDATE users SET display_name = email WHERE display_name = '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_login ON users(login);
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_google_sub ON users(google_sub) WHERE google_sub IS NOT NULL AND google_sub != '';

CREATE TABLE IF NOT EXISTS email_verifications (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at INTEGER NOT NULL,
    created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS oauth_states (
    state TEXT PRIMARY KEY,
    expires_at INTEGER NOT NULL,
    created_at INTEGER NOT NULL
);
