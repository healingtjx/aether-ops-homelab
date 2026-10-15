ALTER TABLE ci_configs
    ADD COLUMN IF NOT EXISTS release_branch VARCHAR(255) NOT NULL DEFAULT 'main';

UPDATE ci_configs cc
SET release_branch = a.default_branch,
    updated_at = NOW()
FROM applications a
WHERE cc.application_id = a.id
  AND cc.release_branch = 'main'
  AND a.default_branch <> '';
