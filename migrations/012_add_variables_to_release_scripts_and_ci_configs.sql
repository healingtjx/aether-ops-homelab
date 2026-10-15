ALTER TABLE release_scripts
    ADD COLUMN IF NOT EXISTS default_variables JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE ci_configs
    ADD COLUMN IF NOT EXISTS variables JSONB NOT NULL DEFAULT '{}'::jsonb;
