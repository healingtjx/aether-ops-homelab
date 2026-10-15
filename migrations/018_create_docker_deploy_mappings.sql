CREATE TABLE IF NOT EXISTS docker_deploy_mappings (
    id                    BIGSERIAL     PRIMARY KEY,
    ci_config_id          BIGINT        NOT NULL REFERENCES ci_configs(id) ON DELETE CASCADE,
    registry_id           BIGINT        NOT NULL REFERENCES artifact_registries(id) ON DELETE RESTRICT,
    repository            VARCHAR(1024) NOT NULL,
    compose_service_name  VARCHAR(255)  NOT NULL,
    remote_dir            VARCHAR(1024) NOT NULL,
    compose_file          VARCHAR(1024) NOT NULL DEFAULT 'docker-compose.yml',
    description           TEXT,
    enabled               BOOLEAN       NOT NULL DEFAULT TRUE,
    last_deploy_status    VARCHAR(50),
    last_deploy_message   TEXT,
    last_deployed_at      TIMESTAMPTZ,
    created_at            TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    UNIQUE(ci_config_id, registry_id, repository, compose_service_name, remote_dir, compose_file)
);

CREATE INDEX IF NOT EXISTS idx_docker_deploy_mappings_ci_config_id ON docker_deploy_mappings(ci_config_id);
CREATE INDEX IF NOT EXISTS idx_docker_deploy_mappings_registry_repository ON docker_deploy_mappings(registry_id, repository);
CREATE INDEX IF NOT EXISTS idx_docker_deploy_mappings_enabled ON docker_deploy_mappings(enabled);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'operation_logs' AND column_name = 'artifact_id'
    ) THEN
        ALTER TABLE operation_logs ADD COLUMN artifact_id BIGINT REFERENCES artifacts(id) ON DELETE SET NULL;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'operation_logs' AND column_name = 'artifact_image'
    ) THEN
        ALTER TABLE operation_logs ADD COLUMN artifact_image VARCHAR(2048);
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'operation_logs' AND column_name = 'compose_service_name'
    ) THEN
        ALTER TABLE operation_logs ADD COLUMN compose_service_name VARCHAR(255);
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'operation_logs' AND column_name = 'deploy_remote_dir'
    ) THEN
        ALTER TABLE operation_logs ADD COLUMN deploy_remote_dir VARCHAR(1024);
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_operation_logs_artifact_id ON operation_logs(artifact_id);
