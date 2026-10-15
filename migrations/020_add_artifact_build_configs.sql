ALTER TABLE ci_configs
    ADD COLUMN IF NOT EXISTS config_type VARCHAR(50) DEFAULT 'ci';

ALTER TABLE ci_configs
    ADD COLUMN IF NOT EXISTS artifact_registry_id BIGINT;

ALTER TABLE ci_configs
    ADD COLUMN IF NOT EXISTS artifact_release_script_id BIGINT;
