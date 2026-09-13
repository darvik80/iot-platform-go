-- 001_init.sql — complete schema for Cogitor IoT Platform
-- All tables, columns, indexes in a single fresh-install migration.

PRAGMA foreign_keys = ON;

-- ── Users & Auth ────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL CHECK(role IN ('admin','operator','viewer')),
    login TEXT,
    display_name TEXT NOT NULL DEFAULT '',
    email_verified INTEGER NOT NULL DEFAULT 0,
    google_sub TEXT,
    disabled INTEGER NOT NULL DEFAULT 0,
    is_platform_admin INTEGER NOT NULL DEFAULT 0,
    avatar_url TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

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

CREATE TABLE IF NOT EXISTS api_tokens (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    token_prefix TEXT NOT NULL,
    account_id INTEGER REFERENCES accounts(id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL,
    last_used_at INTEGER
);

-- ── Accounts (multi-tenancy) ────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS accounts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    slug TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    settings_json TEXT NOT NULL DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'active',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS account_memberships (
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK(role IN ('admin','operator','viewer')),
    status TEXT NOT NULL DEFAULT 'active',
    joined_at INTEGER NOT NULL,
    invited_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
    PRIMARY KEY(user_id, account_id)
);

CREATE INDEX IF NOT EXISTS idx_account_memberships_account ON account_memberships(account_id);

CREATE TABLE IF NOT EXISTS account_plugin_policies (
    account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    plugin_id TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    settings_json TEXT NOT NULL DEFAULT '{}',
    updated_at INTEGER NOT NULL,
    PRIMARY KEY(account_id, plugin_id)
);

CREATE TABLE IF NOT EXISTS account_invitations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    email TEXT NOT NULL,
    role TEXT NOT NULL CHECK(role IN ('admin','operator','viewer')),
    token_hash TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'pending',
    delivery_method TEXT NOT NULL DEFAULT 'email',
    invited_by INTEGER NOT NULL REFERENCES users(id),
    expires_at INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_invitations_account ON account_invitations(account_id);
CREATE INDEX IF NOT EXISTS idx_invitations_token ON account_invitations(token_hash);
CREATE INDEX IF NOT EXISTS idx_invitations_email ON account_invitations(email);

-- ── Products & Devices ──────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS products (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id INTEGER NOT NULL DEFAULT 1 REFERENCES accounts(id) ON DELETE CASCADE,
    product_id TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    topic_style_default TEXT NOT NULL DEFAULT 'short' CHECK(topic_style_default IN ('short','full')),
    methods_json TEXT NOT NULL DEFAULT '[]',
    metrics_json TEXT NOT NULL DEFAULT '[]',
    attributes_json TEXT NOT NULL DEFAULT '[]',
    auto_ota INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_products_account ON products(account_id);

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
    auto_ota INTEGER NOT NULL DEFAULT 1,
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
    account_id INTEGER REFERENCES accounts(id) ON DELETE CASCADE,
    revoked_at INTEGER,
    created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_device_credentials_account ON device_credentials(account_id);

-- ── Telemetry & Attributes ──────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS telemetry_samples (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    device_pk INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    account_id INTEGER REFERENCES accounts(id) ON DELETE CASCADE,
    key TEXT NOT NULL,
    value_num REAL,
    value_text TEXT,
    value_bool INTEGER,
    ts INTEGER NOT NULL,
    ingested_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_telemetry_device_key_ts ON telemetry_samples(device_pk, key, ts);
CREATE INDEX IF NOT EXISTS idx_telemetry_samples_account ON telemetry_samples(account_id);

CREATE TABLE IF NOT EXISTS device_attributes_reported (
    device_pk INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    account_id INTEGER REFERENCES accounts(id) ON DELETE CASCADE,
    key TEXT NOT NULL,
    value_json TEXT NOT NULL,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY(device_pk, key)
);

CREATE INDEX IF NOT EXISTS idx_device_attrs_reported_account ON device_attributes_reported(account_id);

CREATE TABLE IF NOT EXISTS device_attributes_desired (
    device_pk INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    account_id INTEGER REFERENCES accounts(id) ON DELETE CASCADE,
    key TEXT NOT NULL,
    value_json TEXT NOT NULL,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY(device_pk, key)
);

CREATE INDEX IF NOT EXISTS idx_device_attrs_desired_account ON device_attributes_desired(account_id);

CREATE TABLE IF NOT EXISTS attribute_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    device_pk INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    account_id INTEGER REFERENCES accounts(id) ON DELETE CASCADE,
    scope TEXT NOT NULL CHECK(scope IN ('reported','desired')),
    key TEXT NOT NULL,
    value_json TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);

-- ── RPC ─────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS rpc_calls (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    device_pk INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    account_id INTEGER REFERENCES accounts(id) ON DELETE CASCADE,
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
CREATE INDEX IF NOT EXISTS idx_rpc_calls_account ON rpc_calls(account_id);

-- ── Events & Logs ───────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    device_pk INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    account_id INTEGER REFERENCES accounts(id) ON DELETE CASCADE,
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
CREATE INDEX IF NOT EXISTS idx_events_account ON events(account_id);

CREATE TABLE IF NOT EXISTS device_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    device_pk INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    account_id INTEGER REFERENCES accounts(id) ON DELETE CASCADE,
    level TEXT NOT NULL,
    module TEXT NOT NULL DEFAULT '',
    code TEXT NOT NULL DEFAULT '',
    message TEXT NOT NULL DEFAULT '',
    context_json TEXT,
    ts INTEGER NOT NULL,
    ingested_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_logs_device_ts ON device_logs(device_pk, ts DESC);
CREATE INDEX IF NOT EXISTS idx_device_logs_account ON device_logs(account_id);

-- ── OTA ─────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS ota_firmwares (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    product_pk INTEGER NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    account_id INTEGER REFERENCES accounts(id) ON DELETE CASCADE,
    module TEXT NOT NULL DEFAULT 'main',
    version TEXT NOT NULL,
    size INTEGER NOT NULL,
    sha256 TEXT NOT NULL,
    sign_method TEXT NOT NULL DEFAULT '',
    sign TEXT NOT NULL DEFAULT '',
    url TEXT NOT NULL,
    download_token TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    UNIQUE(product_pk, module, version)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_ota_fw_download_token ON ota_firmwares(download_token) WHERE download_token != '';
CREATE INDEX IF NOT EXISTS idx_ota_firmwares_account ON ota_firmwares(account_id);

CREATE TABLE IF NOT EXISTS ota_jobs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    device_pk INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    account_id INTEGER REFERENCES accounts(id) ON DELETE CASCADE,
    firmware_id INTEGER NOT NULL REFERENCES ota_firmwares(id),
    status TEXT NOT NULL DEFAULT 'pending',
    step INTEGER NOT NULL DEFAULT 0,
    description TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_ota_jobs_account ON ota_jobs(account_id);

-- ── Plugins ─────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS device_plugin_bindings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    device_pk INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    plugin_id TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    config_json TEXT NOT NULL DEFAULT '{}',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(device_pk, plugin_id)
);

CREATE INDEX IF NOT EXISTS idx_device_plugins_device ON device_plugin_bindings(device_pk);

CREATE TABLE IF NOT EXISTS external_plugins (
    id                TEXT PRIMARY KEY,
    account_id        INTEGER NOT NULL DEFAULT 1 REFERENCES accounts(id) ON DELETE CASCADE,
    name              TEXT NOT NULL,
    description       TEXT NOT NULL DEFAULT '',
    endpoint          TEXT NOT NULL DEFAULT '',
    auth_token_hash   TEXT NOT NULL,
    mqtt_user         TEXT NOT NULL DEFAULT '',
    mqtt_pass_hash    TEXT NOT NULL DEFAULT '',
    auto              INTEGER NOT NULL DEFAULT 0,
    interval_ms       INTEGER NOT NULL DEFAULT 0,
    default_config    TEXT NOT NULL DEFAULT '{}',
    effects           TEXT NOT NULL DEFAULT '[]',
    form              TEXT NOT NULL DEFAULT '[]',
    mode              TEXT NOT NULL DEFAULT 'webhook',
    status            TEXT NOT NULL DEFAULT 'unknown',
    last_heartbeat_at INTEGER,
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_external_plugins_account ON external_plugins(account_id);

CREATE TABLE IF NOT EXISTS external_plugin_events (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    plugin_id   TEXT NOT NULL REFERENCES external_plugins(id) ON DELETE CASCADE,
    binding_id  INTEGER NOT NULL,
    method      TEXT NOT NULL,
    params      TEXT NOT NULL,
    created_at  INTEGER NOT NULL,
    acked_at    INTEGER
);

CREATE INDEX IF NOT EXISTS idx_external_plugin_events_plugin ON external_plugin_events(plugin_id, acked_at);

-- ── Audit & Settings ────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS audit_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER,
    account_id INTEGER REFERENCES accounts(id) ON DELETE CASCADE,
    actor_kind TEXT NOT NULL DEFAULT 'user',
    action TEXT NOT NULL,
    entity_type TEXT NOT NULL DEFAULT '',
    entity_id TEXT NOT NULL DEFAULT '',
    detail_json TEXT,
    created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_audit_log_account ON audit_log(account_id);

CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- Default Node-RED MQTT credentials
INSERT OR IGNORE INTO settings (key, value) VALUES ('nodered_mqtt_user', 'nodered');
INSERT OR IGNORE INTO settings (key, value) VALUES ('nodered_mqtt_pass', '');

CREATE TABLE IF NOT EXISTS ingest_dedup (
    device_pk INTEGER NOT NULL,
    account_id INTEGER REFERENCES accounts(id) ON DELETE CASCADE,
    capability TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    seen_at INTEGER NOT NULL,
    PRIMARY KEY(device_pk, capability, fingerprint)
);
