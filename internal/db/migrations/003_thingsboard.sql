-- 003_thingsboard.sql — ThingsBoard dialect (TB-compatible devices)
--
-- topic_style gains 'thingsboard'. No new columns: TB devices use the
-- standard access-token credential (username = token, no password).
--
-- SQLite cannot ALTER a CHECK constraint, so both tables are rebuilt
-- (the documented 12-step recipe). PRAGMA foreign_keys=OFF must run
-- OUTSIDE the migration transaction — the engine detects it below and
-- applies it on the connection around the tx (a PRAGMA inside a tx is
-- a silent no-op).

PRAGMA foreign_keys=OFF;

-- sqlite:begin
CREATE TABLE products_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    product_id TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    topic_style_default TEXT NOT NULL DEFAULT 'short' CHECK(topic_style_default IN ('short','full','alink','thingsboard')),
    methods_json TEXT NOT NULL DEFAULT '[]',
    metrics_json TEXT NOT NULL DEFAULT '[]',
    attributes_json TEXT NOT NULL DEFAULT '[]',
    auto_ota INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(account_id, product_id)
);
INSERT INTO products_new(id, account_id, product_id, name, description, topic_style_default, methods_json, metrics_json, attributes_json, auto_ota, created_at, updated_at)
    SELECT id, account_id, product_id, name, description, topic_style_default, methods_json, metrics_json, attributes_json, auto_ota, created_at, updated_at FROM products;
DROP TABLE products;
ALTER TABLE products_new RENAME TO products;
CREATE INDEX IF NOT EXISTS idx_products_account ON products(account_id);

CREATE TABLE devices_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    product_pk INTEGER NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    device_id TEXT NOT NULL,
    name TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    tags TEXT NOT NULL DEFAULT '[]',
    access_token_hash TEXT,
    token_prefix TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'unknown' CHECK(status IN ('online','offline','unknown')),
    last_seen_at INTEGER,
    last_ip TEXT NOT NULL DEFAULT '',
    fw_version TEXT NOT NULL DEFAULT '',
    topic_style TEXT NOT NULL DEFAULT 'short' CHECK(topic_style IN ('short','full','alink','thingsboard')),
    alink_secret TEXT,
    auto_ota INTEGER NOT NULL DEFAULT 1,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(product_pk, device_id)
);
INSERT INTO devices_new(id, product_pk, device_id, name, description, tags, access_token_hash, token_prefix, status, last_seen_at, last_ip, fw_version, topic_style, alink_secret, auto_ota, enabled, created_at, updated_at)
    SELECT id, product_pk, device_id, name, description, tags, access_token_hash, token_prefix, status, last_seen_at, last_ip, fw_version, topic_style, alink_secret, auto_ota, enabled, created_at, updated_at FROM devices;
DROP TABLE devices;
ALTER TABLE devices_new RENAME TO devices;
CREATE INDEX IF NOT EXISTS idx_devices_status ON devices(status);
CREATE INDEX IF NOT EXISTS idx_devices_token_prefix ON devices(token_prefix);

PRAGMA foreign_keys=ON;
-- sqlite:end

-- pg:begin
ALTER TABLE devices DROP CONSTRAINT IF EXISTS devices_topic_style_check;
ALTER TABLE devices ADD CONSTRAINT devices_topic_style_check CHECK(topic_style IN ('short','full','alink','thingsboard'));
ALTER TABLE products DROP CONSTRAINT IF EXISTS products_topic_style_default_check;
ALTER TABLE products ADD CONSTRAINT products_topic_style_default_check CHECK(topic_style_default IN ('short','full','alink','thingsboard'));
-- pg:end
