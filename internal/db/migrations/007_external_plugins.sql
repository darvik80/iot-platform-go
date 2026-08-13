-- External plugins registry
CREATE TABLE IF NOT EXISTS external_plugins (
    id                TEXT PRIMARY KEY,
    name              TEXT NOT NULL,
    description       TEXT NOT NULL DEFAULT '',
    endpoint          TEXT NOT NULL DEFAULT '',  -- webhook URL (empty = polling mode)
    auth_token_hash   TEXT NOT NULL,
    auto              INTEGER NOT NULL DEFAULT 0,
    interval_ms       INTEGER NOT NULL DEFAULT 0,
    default_config    TEXT NOT NULL DEFAULT '{}',
    effects           TEXT NOT NULL DEFAULT '[]',
    form              TEXT NOT NULL DEFAULT '[]',
    mode              TEXT NOT NULL DEFAULT 'webhook',  -- webhook | polling
    status            TEXT NOT NULL DEFAULT 'unknown',  -- online | offline | unknown
    last_heartbeat_at INTEGER,
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL
);

-- Event queue for polling mode
CREATE TABLE IF NOT EXISTS external_plugin_events (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    plugin_id   TEXT NOT NULL REFERENCES external_plugins(id) ON DELETE CASCADE,
    binding_id  INTEGER NOT NULL,
    method      TEXT NOT NULL,  -- plugin.apply_config | plugin.on_event | plugin.detach
    params      TEXT NOT NULL,  -- JSON
    created_at  INTEGER NOT NULL,
    acked_at    INTEGER         -- NULL = not acknowledged
);

CREATE INDEX IF NOT EXISTS idx_external_plugin_events_plugin ON external_plugin_events(plugin_id, acked_at);
