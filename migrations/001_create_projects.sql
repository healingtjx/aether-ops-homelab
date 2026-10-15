CREATE TABLE IF NOT EXISTS projects (
    id          BIGSERIAL       PRIMARY KEY,
    name        VARCHAR(255)    NOT NULL UNIQUE,
    description TEXT,
    created_at  TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ     NOT NULL DEFAULT NOW()
);
