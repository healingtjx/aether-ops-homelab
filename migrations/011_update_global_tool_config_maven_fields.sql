ALTER TABLE global_tool_config
    ADD COLUMN IF NOT EXISTS maven_home_path VARCHAR(1024) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS maven_user_settings_file VARCHAR(1024) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS maven_local_repository VARCHAR(1024) NOT NULL DEFAULT '';

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = 'global_tool_config'
          AND column_name = 'maven_path'
    ) THEN
        EXECUTE 'UPDATE global_tool_config
                 SET maven_home_path = regexp_replace(maven_path, ''/bin/mvn$'', '''')
                 WHERE maven_home_path = ''''';
    END IF;
END $$;

ALTER TABLE global_tool_config
    DROP COLUMN IF EXISTS maven_path;
