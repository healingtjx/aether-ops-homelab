ALTER TABLE ci_configs
    ADD build_mode VARCHAR(50) DEFAULT 'docker' NOT NULL;

ALTER TABLE ci_configs
    ADD CONSTRAINT ci_configs_build_mode_check
    CHECK (build_mode IN ('docker', 'script'));
