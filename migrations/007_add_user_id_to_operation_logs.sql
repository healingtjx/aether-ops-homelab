ALTER TABLE operation_logs ADD COLUMN IF NOT EXISTS user_id BIGINT REFERENCES users(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_operation_logs_user_id ON operation_logs(user_id);
