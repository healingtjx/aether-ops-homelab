CREATE TABLE IF NOT EXISTS operation_logs (
    id              BIGSERIAL       PRIMARY KEY,
    application_id  BIGINT          NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    project_id      BIGINT          NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    operation_type  VARCHAR(50)     NOT NULL,
    status          VARCHAR(50)     NOT NULL DEFAULT 'running',
    message         TEXT,
    started_at      TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    finished_at     TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_operation_logs_app_id ON operation_logs(application_id);
CREATE INDEX IF NOT EXISTS idx_operation_logs_project_id ON operation_logs(project_id);
CREATE INDEX IF NOT EXISTS idx_operation_logs_started_at ON operation_logs(started_at DESC);
