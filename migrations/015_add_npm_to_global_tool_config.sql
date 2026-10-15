DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = 'global_tool_config'
          AND column_name = 'npm_home_path'
    ) THEN
        ALTER TABLE global_tool_config
            ADD COLUMN npm_home_path VARCHAR(1024) NOT NULL DEFAULT '';
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = 'global_tool_config'
          AND column_name = 'npm_user_config_file'
    ) THEN
        ALTER TABLE global_tool_config
            ADD COLUMN npm_user_config_file VARCHAR(1024) NOT NULL DEFAULT '';
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = 'global_tool_config'
          AND column_name = 'npm_cache_directory'
    ) THEN
        ALTER TABLE global_tool_config
            ADD COLUMN npm_cache_directory VARCHAR(1024) NOT NULL DEFAULT '';
    END IF;
END $$;
