CREATE TABLE IF NOT EXISTS applications (
    id                BIGSERIAL       PRIMARY KEY,
    project_id        BIGINT          NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name              VARCHAR(255)    NOT NULL,
    description       TEXT,
    git_url           VARCHAR(1024)   NOT NULL,
    default_branch    VARCHAR(255)    NOT NULL DEFAULT 'main',
    git_credential_id BIGINT          REFERENCES git_credentials(id) ON DELETE SET NULL,
    created_at        TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    UNIQUE(project_id, name)
);

CREATE INDEX IF NOT EXISTS idx_applications_project_id ON applications(project_id);
CREATE INDEX IF NOT EXISTS idx_applications_git_credential_id ON applications(git_credential_id);
