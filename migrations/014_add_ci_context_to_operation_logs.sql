DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'operation_logs' AND column_name = 'ci_config_id'
    ) THEN
        ALTER TABLE operation_logs ADD COLUMN ci_config_id BIGINT REFERENCES ci_configs(id) ON DELETE SET NULL;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'operation_logs' AND column_name = 'release_script_id'
    ) THEN
        ALTER TABLE operation_logs ADD COLUMN release_script_id BIGINT REFERENCES release_scripts(id) ON DELETE SET NULL;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'operation_logs' AND column_name = 'release_script_name'
    ) THEN
        ALTER TABLE operation_logs ADD COLUMN release_script_name VARCHAR(255);
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'operation_logs' AND column_name = 'release_branch'
    ) THEN
        ALTER TABLE operation_logs ADD COLUMN release_branch VARCHAR(255);
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'operation_logs' AND column_name = 'module_name'
    ) THEN
        ALTER TABLE operation_logs ADD COLUMN module_name VARCHAR(255);
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'operation_logs' AND column_name = 'module_path'
    ) THEN
        ALTER TABLE operation_logs ADD COLUMN module_path VARCHAR(1024);
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_operation_logs_ci_config_id ON operation_logs(ci_config_id);
CREATE INDEX IF NOT EXISTS idx_operation_logs_release_branch ON operation_logs(release_branch);
CREATE INDEX IF NOT EXISTS idx_operation_logs_module_name ON operation_logs(module_name);
