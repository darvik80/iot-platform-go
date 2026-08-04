-- +migrate Up
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL CHECK(role IN ('admin','operator','viewer')),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS api_tokens (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    token_prefix TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    last_used_at INTEGER
);

CREATE TABLE IF NOT EXISTS products (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    product_id TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    topic_style_default TEXT NOT NULL DEFAULT 'short' CHECK(topic_style_default IN ('short','full')),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS devices (
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
    topic_style TEXT NOT NULL DEFAULT 'short' CHECK(topic_style IN ('short','full')),
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(product_pk, device_id)
);

CREATE INDEX IF NOT EXISTS idx_devices_status ON devices(status);
CREATE INDEX IF NOT EXISTS idx_devices_token_prefix ON devices(token_prefix);

CREATE TABLE IF NOT EXISTS device_credentials (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    device_pk INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL,
    token_prefix TEXT NOT NULL,
    revoked_at INTEGER,
    created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS telemetry_samples (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    device_pk INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    key TEXT NOT NULL,
    value_num REAL,
    value_text TEXT,
    value_bool INTEGER,
    ts INTEGER NOT NULL,
    ingested_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_telemetry_device_key_ts ON telemetry_samples(device_pk, key, ts);

CREATE TABLE IF NOT EXISTS device_attributes_reported (
    device_pk INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    key TEXT NOT NULL,
    value_json TEXT NOT NULL,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY(device_pk, key)
);

CREATE TABLE IF NOT EXISTS device_attributes_desired (
    device_pk INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    key TEXT NOT NULL,
    value_json TEXT NOT NULL,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY(device_pk, key)
);

CREATE TABLE IF NOT EXISTS attribute_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    device_pk INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    scope TEXT NOT NULL CHECK(scope IN ('reported','desired')),
    key TEXT NOT NULL,
    value_json TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS rpc_calls (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    device_pk INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    request_id INTEGER NOT NULL,
    method TEXT NOT NULL,
    params_json TEXT NOT NULL DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','ok','error','timeout')),
    code INTEGER,
    message TEXT NOT NULL DEFAULT '',
    data_json TEXT,
    correlation_id TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    completed_at INTEGER
);

CREATE INDEX IF NOT EXISTS idx_rpc_device_created ON rpc_calls(device_pk, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_rpc_pending ON rpc_calls(status, created_at);

CREATE TABLE IF NOT EXISTS events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    device_pk INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    event_id TEXT NOT NULL DEFAULT '',
    type TEXT NOT NULL,
    severity TEXT NOT NULL DEFAULT 'info',
    code TEXT NOT NULL,
    message TEXT NOT NULL DEFAULT '',
    data_json TEXT,
    ts INTEGER NOT NULL,
    ingested_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_events_device_ts ON events(device_pk, ts DESC);

CREATE TABLE IF NOT EXISTS device_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    device_pk INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    level TEXT NOT NULL,
    module TEXT NOT NULL DEFAULT '',
    code TEXT NOT NULL DEFAULT '',
    message TEXT NOT NULL DEFAULT '',
    context_json TEXT,
    ts INTEGER NOT NULL,
    ingested_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_logs_device_ts ON device_logs(device_pk, ts DESC);

CREATE TABLE IF NOT EXISTS ota_firmwares (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    product_pk INTEGER NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    module TEXT NOT NULL DEFAULT 'main',
    version TEXT NOT NULL,
    size INTEGER NOT NULL,
    sha256 TEXT NOT NULL,
    sign_method TEXT NOT NULL DEFAULT '',
    sign TEXT NOT NULL DEFAULT '',
    url TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    UNIQUE(product_pk, module, version)
);

CREATE TABLE IF NOT EXISTS ota_jobs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    device_pk INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    firmware_id INTEGER NOT NULL REFERENCES ota_firmwares(id),
    status TEXT NOT NULL DEFAULT 'pending',
    step INTEGER NOT NULL DEFAULT 0,
    desc TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS audit_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER,
    action TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL DEFAULT '',
    detail_json TEXT,
    created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS ingest_dedup (
    device_pk INTEGER NOT NULL,
    capability TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    seen_at INTEGER NOT NULL,
    PRIMARY KEY(device_pk, capability, fingerprint)
);
