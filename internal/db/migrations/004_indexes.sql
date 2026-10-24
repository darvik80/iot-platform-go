-- 004_indexes.sql — Add missing indexes for retention and list queries

-- Retention deletes by updated_at (was full table scan)
CREATE INDEX IF NOT EXISTS idx_attribute_history_updated_at ON attribute_history(updated_at);

-- Audit log list queries ordered by created_at DESC
CREATE INDEX IF NOT EXISTS idx_audit_log_created_at ON audit_log(created_at);

-- Device log list queries ordered by ts DESC
CREATE INDEX IF NOT EXISTS idx_device_logs_ts ON device_logs(ts);
