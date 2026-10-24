CREATE TABLE monitoring_channels (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    kind TEXT NOT NULL CHECK(kind IN ('email','telegram')),
    destination TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0,1)),
    secret_ciphertext TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
CREATE INDEX idx_monitoring_channels_account ON monitoring_channels(account_id, id);

CREATE TABLE monitoring_rules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    device_id INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    metric TEXT NOT NULL DEFAULT '',
    kind TEXT NOT NULL CHECK(kind IN ('threshold','no_data','offline')),
    operator TEXT NOT NULL DEFAULT '',
    threshold DOUBLE PRECISION NOT NULL DEFAULT 0,
    recovery DOUBLE PRECISION NOT NULL DEFAULT 0,
    for_seconds INTEGER NOT NULL DEFAULT 0,
    missing_seconds INTEGER NOT NULL,
    cooldown_seconds INTEGER NOT NULL CHECK(cooldown_seconds >= 60),
    enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0,1)),
    channel_ids TEXT NOT NULL DEFAULT '[]',
    state TEXT NOT NULL DEFAULT 'normal' CHECK(state IN ('normal','pending','firing','resolved')),
    since INTEGER NOT NULL,
    last_notified INTEGER NOT NULL DEFAULT 0,
    last_evaluated INTEGER NOT NULL DEFAULT 0,
    revision INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
CREATE INDEX idx_monitoring_rules_account ON monitoring_rules(account_id, id);
CREATE INDEX idx_monitoring_rules_enabled ON monitoring_rules(enabled, id);

CREATE TABLE monitoring_alert_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    rule_id INTEGER NOT NULL REFERENCES monitoring_rules(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    state TEXT NOT NULL CHECK(state IN ('normal','pending','firing','resolved')),
    message TEXT NOT NULL,
    created_at INTEGER NOT NULL
);
CREATE INDEX idx_monitoring_events_account ON monitoring_alert_events(account_id, id);

CREATE TABLE monitoring_deliveries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    event_id INTEGER NOT NULL REFERENCES monitoring_alert_events(id) ON DELETE CASCADE,
    channel_id INTEGER NOT NULL REFERENCES monitoring_channels(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','sending','sent','failed')),
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    next_attempt INTEGER NOT NULL,
    lease_until INTEGER NOT NULL DEFAULT 0,
    lease_token TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(event_id, channel_id)
);
CREATE INDEX idx_monitoring_deliveries_account ON monitoring_deliveries(account_id, id);
CREATE INDEX idx_monitoring_deliveries_due ON monitoring_deliveries(status, next_attempt, lease_until);

-- Latest server-received samples, with and without a metric filter.
CREATE INDEX idx_monitoring_telemetry_received ON telemetry_samples(account_id, device_pk, ingested_at, id);
CREATE INDEX idx_monitoring_telemetry_metric_received ON telemetry_samples(account_id, device_pk, key, ingested_at, id);
