DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'artifact_registries' AND column_name = 'repository_names'
    ) THEN
        ALTER TABLE artifact_registries ADD COLUMN repository_names TEXT NOT NULL DEFAULT '';
    END IF;
END $$;
