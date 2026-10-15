DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = 'ci_configs'
          AND column_name = 'module_name'
    ) THEN
        ALTER TABLE ci_configs ADD COLUMN module_name VARCHAR(255);
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = 'ci_configs'
          AND column_name = 'module_path'
    ) THEN
        ALTER TABLE ci_configs ADD COLUMN module_path VARCHAR(1024);
    END IF;
END $$;

UPDATE ci_configs cc
SET module_name = a.name,
    module_path = a.name
FROM applications a
WHERE cc.application_id = a.id
  AND (cc.module_name IS NULL OR cc.module_name = '');

UPDATE ci_configs
SET module_path = module_name
WHERE module_path IS NULL OR module_path = '';

ALTER TABLE ci_configs
    ALTER COLUMN module_name SET NOT NULL,
    ALTER COLUMN module_path SET NOT NULL;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conrelid = 'ci_configs'::regclass
          AND conname = 'ci_configs_application_id_release_script_id_key'
    ) THEN
        ALTER TABLE ci_configs DROP CONSTRAINT ci_configs_application_id_release_script_id_key;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_indexes
        WHERE tablename = 'ci_configs'
          AND indexname = 'idx_ci_configs_application_script_module'
    ) THEN
        CREATE UNIQUE INDEX idx_ci_configs_application_script_module
            ON ci_configs(application_id, release_script_id, module_name);
    END IF;
END $$;
