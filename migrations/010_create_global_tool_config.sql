CREATE TABLE IF NOT EXISTS global_tool_config (
    id                       SMALLINT      PRIMARY KEY DEFAULT 1,
    git_path                 VARCHAR(1024) NOT NULL DEFAULT 'git',
    maven_home_path          VARCHAR(1024) NOT NULL DEFAULT '',
    maven_user_settings_file VARCHAR(1024) NOT NULL DEFAULT '',
    maven_local_repository   VARCHAR(1024) NOT NULL DEFAULT '',
    created_at               TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT global_tool_config_singleton CHECK (id = 1)
);

INSERT INTO global_tool_config (id, git_path, maven_home_path, maven_user_settings_file, maven_local_repository)
VALUES (1, 'git', '', '', '')
ON CONFLICT (id) DO NOTHING;
