CREATE TABLE IF NOT EXISTS git_credentials (
    id              BIGSERIAL       PRIMARY KEY,
    name            VARCHAR(255)    NOT NULL,
    type            VARCHAR(50)     NOT NULL CHECK (type IN ('ssh', 'https')),
    username        VARCHAR(255),
    password        TEXT,
    ssh_private_key TEXT,
    ssh_passphrase  TEXT,
    created_at      TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ     NOT NULL DEFAULT NOW()
);
