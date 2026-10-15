CREATE TABLE IF NOT EXISTS artifact_registries (
    id                 BIGSERIAL     PRIMARY KEY,
    name               VARCHAR(255)  NOT NULL UNIQUE,
    provider           VARCHAR(50)   NOT NULL DEFAULT 'generic',
    registry_url       VARCHAR(1024) NOT NULL,
    namespace          VARCHAR(255)  NOT NULL DEFAULT '',
    auth_type          VARCHAR(50)   NOT NULL DEFAULT 'basic',
    username           VARCHAR(255)  NOT NULL DEFAULT '',
    credential         TEXT          NOT NULL DEFAULT '',
    enabled            BOOLEAN       NOT NULL DEFAULT TRUE,
    last_sync_status   VARCHAR(50),
    last_sync_message  TEXT,
    last_synced_at     TIMESTAMPTZ,
    created_at         TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT artifact_registries_provider_check CHECK (provider IN ('generic', 'aws_ecr', 'aliyun_acr')),
    CONSTRAINT artifact_registries_auth_type_check CHECK (auth_type IN ('basic', 'bearer'))
);

CREATE TABLE IF NOT EXISTS artifacts (
    id              BIGSERIAL     PRIMARY KEY,
    registry_id     BIGINT        NOT NULL REFERENCES artifact_registries(id) ON DELETE CASCADE,
    repository      VARCHAR(1024) NOT NULL,
    tag             VARCHAR(255)  NOT NULL,
    digest          VARCHAR(255),
    media_type      VARCHAR(255),
    size_bytes      BIGINT,
    pushed_at       TIMESTAMPTZ,
    remote_status   VARCHAR(50)   NOT NULL DEFAULT 'present',
    last_synced_at  TIMESTAMPTZ,
    deleted_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    UNIQUE(registry_id, repository, tag)
);

CREATE TABLE IF NOT EXISTS artifact_operation_logs (
    id             BIGSERIAL    PRIMARY KEY,
    registry_id    BIGINT       REFERENCES artifact_registries(id) ON DELETE SET NULL,
    artifact_id     BIGINT       REFERENCES artifacts(id) ON DELETE SET NULL,
    operation_type  VARCHAR(50)  NOT NULL,
    status          VARCHAR(50)  NOT NULL DEFAULT 'running',
    message         TEXT,
    started_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    finished_at     TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_artifacts_registry_id ON artifacts(registry_id);
CREATE INDEX IF NOT EXISTS idx_artifacts_repository ON artifacts(repository);
CREATE INDEX IF NOT EXISTS idx_artifacts_tag ON artifacts(tag);
CREATE INDEX IF NOT EXISTS idx_artifacts_remote_status ON artifacts(remote_status);
CREATE INDEX IF NOT EXISTS idx_artifact_operation_logs_registry_id ON artifact_operation_logs(registry_id);
CREATE INDEX IF NOT EXISTS idx_artifact_operation_logs_started_at ON artifact_operation_logs(started_at DESC);
