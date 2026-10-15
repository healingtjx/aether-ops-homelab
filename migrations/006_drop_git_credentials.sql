-- Drop git credential references from applications
ALTER TABLE applications DROP COLUMN IF EXISTS git_credential_id;
DROP INDEX IF EXISTS idx_applications_git_credential_id;

-- Drop git credentials table
DROP TABLE IF EXISTS git_credentials;
