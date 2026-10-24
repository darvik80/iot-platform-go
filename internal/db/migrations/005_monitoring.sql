CREATE TABLE monitoring_dashboards (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
CREATE INDEX idx_monitoring_dashboards_account ON monitoring_dashboards(account_id, id);
CREATE TABLE monitoring_widgets (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    dashboard_id INTEGER NOT NULL REFERENCES monitoring_dashboards(id) ON DELETE CASCADE,
    position INTEGER NOT NULL,
    config_json TEXT NOT NULL,
    UNIQUE(dashboard_id, position)
);
