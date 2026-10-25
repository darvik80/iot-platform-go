-- Device groups for monitoring, automation, and bulk operations
CREATE TABLE device_groups (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    scope_type TEXT NOT NULL CHECK(scope_type IN ('manual','product','tag')),
    scope_filter TEXT NOT NULL DEFAULT '{}',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
CREATE INDEX idx_device_groups_account ON device_groups(account_id, id);

-- Manual group membership (for scope_type='manual')
CREATE TABLE device_group_members (
    group_id INTEGER NOT NULL REFERENCES device_groups(id) ON DELETE CASCADE,
    device_id INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    PRIMARY KEY(group_id, device_id)
);
CREATE INDEX idx_device_group_members_device ON device_group_members(device_id);

-- Add scope support to monitoring rules
ALTER TABLE monitoring_rules ADD COLUMN scope_json TEXT NOT NULL DEFAULT '';
