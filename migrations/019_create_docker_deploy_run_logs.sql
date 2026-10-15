CREATE TABLE IF NOT EXISTS docker_deploy_runs (
    id                    BIGSERIAL     PRIMARY KEY,
    mapping_id            BIGINT        NOT NULL REFERENCES docker_deploy_mappings(id) ON DELETE CASCADE,
    operation_log_id      BIGINT        REFERENCES operation_logs(id) ON DELETE SET NULL,
    artifact_id           BIGINT        REFERENCES artifacts(id) ON DELETE SET NULL,
    artifact_image        VARCHAR(2048) NOT NULL,
    status                VARCHAR(50)   NOT NULL DEFAULT 'running',
    message               TEXT,
    started_at            TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    finished_at           TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS docker_deploy_run_logs (
    id          BIGSERIAL   PRIMARY KEY,
    run_id      BIGINT      NOT NULL REFERENCES docker_deploy_runs(id) ON DELETE CASCADE,
    line_no     INTEGER     NOT NULL,
    content     TEXT        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(run_id, line_no)
);

CREATE INDEX IF NOT EXISTS idx_docker_deploy_runs_mapping_started ON docker_deploy_runs(mapping_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_docker_deploy_runs_operation_log_id ON docker_deploy_runs(operation_log_id);
CREATE INDEX IF NOT EXISTS idx_docker_deploy_run_logs_run_line ON docker_deploy_run_logs(run_id, line_no);
