-- +migrate Up
-- Multi-account support: accounts, memberships, and account_id propagation

-- Accounts (tenants)
CREATE TABLE IF NOT EXISTS accounts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    slug TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    settings_json TEXT NOT NULL DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'active',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

-- Per-account user membership
CREATE TABLE IF NOT EXISTS account_memberships (
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK(role IN ('admin','operator','viewer')),
    status TEXT NOT NULL DEFAULT 'active',
    joined_at INTEGER NOT NULL,
    invited_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
    PRIMARY KEY(user_id, account_id)
);

-- Per-account plugin policies
CREATE TABLE IF NOT EXISTS account_plugin_policies (
    account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    plugin_id TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    settings_json TEXT NOT NULL DEFAULT '{}',
    updated_at INTEGER NOT NULL,
    PRIMARY KEY(account_id, plugin_id)
);

-- account_id on per-account tables (default to 1 for existing data)
ALTER TABLE products ADD COLUMN account_id INTEGER NOT NULL DEFAULT 1 REFERENCES accounts(id) ON DELETE CASCADE;
ALTER TABLE external_plugins ADD COLUMN account_id INTEGER NOT NULL DEFAULT 1 REFERENCES accounts(id) ON DELETE CASCADE;
ALTER TABLE api_tokens ADD COLUMN account_id INTEGER REFERENCES accounts(id) ON DELETE CASCADE;

-- Platform admin flag on users
ALTER TABLE users ADD COLUMN is_platform_admin INTEGER NOT NULL DEFAULT 0;

-- Index for account lookups
CREATE INDEX IF NOT EXISTS idx_products_account ON products(account_id);
CREATE INDEX IF NOT EXISTS idx_external_plugins_account ON external_plugins(account_id);
CREATE INDEX IF NOT EXISTS idx_account_memberships_account ON account_memberships(account_id);
