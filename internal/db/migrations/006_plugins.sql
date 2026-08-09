-- +migrate Up

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
