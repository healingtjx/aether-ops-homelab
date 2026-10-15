CREATE TABLE IF NOT EXISTS release_scripts (
    id          BIGSERIAL    PRIMARY KEY,
    name        VARCHAR(255) NOT NULL UNIQUE,
    description TEXT,
    content     TEXT         NOT NULL,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS ci_configs (
    id                BIGSERIAL    PRIMARY KEY,
    application_id    BIGINT       NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    release_script_id BIGINT       NOT NULL REFERENCES release_scripts(id) ON DELETE RESTRICT,
    release_branch    VARCHAR(255) NOT NULL DEFAULT 'main',
    last_run_status   VARCHAR(50),
    last_run_message  TEXT,
    last_run_at       TIMESTAMPTZ,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE(application_id, release_script_id)
);

CREATE INDEX IF NOT EXISTS idx_ci_configs_application_id ON ci_configs(application_id);
