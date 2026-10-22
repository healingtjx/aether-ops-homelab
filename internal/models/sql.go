package models

import (
	"fmt"
	"strings"
)

const (
	sqlListProjects = `
SELECT id, name, description, created_at, updated_at
FROM projects
ORDER BY name`

	sqlGetProject = `
SELECT id, name, description, created_at, updated_at
FROM projects
WHERE id = $1`

	sqlCreateProject = `
INSERT INTO projects (name, description)
VALUES ($1, $2)
RETURNING id, name, description, created_at, updated_at`

	sqlUpdateProject = `
UPDATE projects
SET name = $2,
    description = $3,
    updated_at = NOW()
WHERE id = $1
RETURNING id, name, description, created_at, updated_at`

	sqlDeleteProject = `
DELETE FROM projects
WHERE id = $1`

	sqlProjectNameExists = `
SELECT EXISTS (
    SELECT 1
    FROM projects
    WHERE name = $1
      AND id IS DISTINCT FROM $2
)`
)

const (
	sqlListApplications = `
SELECT a.id, a.project_id, p.name AS project_name,
       a.name, a.description, a.git_url, a.default_branch,
       a.last_pulled_at, a.last_pull_status, a.last_pull_message,
       a.created_at, a.updated_at
FROM applications a
JOIN projects p ON a.project_id = p.id
ORDER BY p.name, a.name`

	sqlGetApplication = `
SELECT a.id, a.project_id, p.name AS project_name,
       a.name, a.description, a.git_url, a.default_branch,
       a.last_pulled_at, a.last_pull_status, a.last_pull_message,
       a.created_at, a.updated_at
FROM applications a
JOIN projects p ON a.project_id = p.id
WHERE a.id = $1`

	sqlCreateApplication = `
INSERT INTO applications (project_id, name, description, git_url, default_branch)
VALUES ($1, $2, $3, $4, $5)
RETURNING id`

	sqlUpdateApplication = `
UPDATE applications
SET project_id = $2,
    name = $3,
    description = $4,
    git_url = $5,
    default_branch = $6,
    updated_at = NOW()
WHERE id = $1
RETURNING id`

	sqlDeleteApplication = `
DELETE FROM applications
WHERE id = $1`

	sqlUpdateApplicationBranch = `
UPDATE applications
SET default_branch = $2,
    updated_at = NOW()
WHERE id = $1`

	sqlUpdateApplicationPullStatusSuccess = `
UPDATE applications
SET last_pull_status = $2,
    last_pull_message = $3,
    last_pulled_at = NOW(),
    updated_at = NOW()
WHERE id = $1`

	sqlUpdateApplicationPullStatus = `
UPDATE applications
SET last_pull_status = $2,
    last_pull_message = $3,
    updated_at = NOW()
WHERE id = $1`
)

const (
	sqlListReleaseScripts = `
SELECT id, name, description, content, default_variables, created_at, updated_at
FROM release_scripts
ORDER BY name`

	sqlGetReleaseScript = `
SELECT id, name, description, content, default_variables, created_at, updated_at
FROM release_scripts
WHERE id = $1`

	sqlCreateReleaseScript = `
INSERT INTO release_scripts (name, description, content, default_variables)
VALUES ($1, $2, $3, $4)
RETURNING id`

	sqlUpdateReleaseScript = `
UPDATE release_scripts
SET name = $2,
    description = $3,
    content = $4,
    default_variables = $5,
    updated_at = NOW()
WHERE id = $1`

	sqlDeleteReleaseScript = `
DELETE FROM release_scripts
WHERE id = $1`
)

const (
	sqlListAllCIConfigs = `
SELECT cc.id, cc.config_type, cc.build_mode, cc.application_id, a.name AS application_name,
       a.project_id, p.name AS project_name,
       cc.release_script_id, rs.name AS release_script_name,
       cc.artifact_registry_id, ar.name AS artifact_registry_name,
       cc.artifact_release_script_id, ars.name AS artifact_release_script_name,
       cc.release_branch, cc.module_name, cc.module_path, cc.variables,
       cc.last_run_status, cc.last_run_message, cc.last_run_at,
       cc.created_at, cc.updated_at
FROM ci_configs cc
JOIN applications a ON cc.application_id = a.id
JOIN projects p ON a.project_id = p.id
JOIN release_scripts rs ON cc.release_script_id = rs.id
LEFT JOIN artifact_registries ar ON cc.artifact_registry_id = ar.id
LEFT JOIN release_scripts ars ON cc.artifact_release_script_id = ars.id
ORDER BY p.name, a.name, cc.release_branch, rs.name, cc.module_name`

	sqlListCIConfigsByType = `
SELECT cc.id, cc.config_type, cc.build_mode, cc.application_id, a.name AS application_name,
       a.project_id, p.name AS project_name,
       cc.release_script_id, rs.name AS release_script_name,
       cc.artifact_registry_id, ar.name AS artifact_registry_name,
       cc.artifact_release_script_id, ars.name AS artifact_release_script_name,
       cc.release_branch, cc.module_name, cc.module_path, cc.variables,
       cc.last_run_status, cc.last_run_message, cc.last_run_at,
       cc.created_at, cc.updated_at
FROM ci_configs cc
JOIN applications a ON cc.application_id = a.id
JOIN projects p ON a.project_id = p.id
JOIN release_scripts rs ON cc.release_script_id = rs.id
LEFT JOIN artifact_registries ar ON cc.artifact_registry_id = ar.id
LEFT JOIN release_scripts ars ON cc.artifact_release_script_id = ars.id
WHERE cc.config_type = $1
ORDER BY p.name, a.name, cc.release_branch, rs.name, cc.module_name`

	sqlGetCIConfig = `
SELECT cc.id, cc.config_type, cc.build_mode, cc.application_id, a.name AS application_name,
       a.project_id, p.name AS project_name,
       cc.release_script_id, rs.name AS release_script_name,
       cc.artifact_registry_id, ar.name AS artifact_registry_name,
       cc.artifact_release_script_id, ars.name AS artifact_release_script_name,
       cc.release_branch, cc.module_name, cc.module_path, cc.variables,
       cc.last_run_status, cc.last_run_message, cc.last_run_at,
       cc.created_at, cc.updated_at
FROM ci_configs cc
JOIN applications a ON cc.application_id = a.id
JOIN projects p ON a.project_id = p.id
JOIN release_scripts rs ON cc.release_script_id = rs.id
LEFT JOIN artifact_registries ar ON cc.artifact_registry_id = ar.id
LEFT JOIN release_scripts ars ON cc.artifact_release_script_id = ars.id
WHERE cc.id = $1`

	sqlCreateCIConfig = `
INSERT INTO ci_configs (config_type, build_mode, application_id, release_script_id, artifact_registry_id, artifact_release_script_id, release_branch, module_name, module_path, variables)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING id`

	sqlUpdateCIConfig = `
UPDATE ci_configs
SET release_script_id = $2,
    release_branch = $3,
    module_name = $4,
    module_path = $5,
    variables = $6,
    updated_at = NOW()
WHERE id = $1
RETURNING application_id`

	sqlRefreshArtifactBuildConfigFromApplication = `
UPDATE ci_configs cc
SET release_branch = COALESCE(NULLIF(a.default_branch, ''), 'main'),
    updated_at = NOW()
FROM applications a
WHERE cc.application_id = a.id
  AND cc.id = $1
  AND cc.config_type = 'artifact'
RETURNING cc.id`

	sqlUpdateArtifactBuildSettings = `
UPDATE ci_configs
SET artifact_release_script_id = $2,
    build_mode = $3,
    artifact_registry_id = $4,
    updated_at = NOW()
WHERE id = $1
  AND config_type = 'artifact'
RETURNING id`

	sqlDeleteCIConfig = `
DELETE FROM ci_configs
WHERE id = $1`

	sqlUpdateCIRunStatusSuccess = `
UPDATE ci_configs
SET last_run_status = $2,
    last_run_message = $3,
    last_run_at = NOW(),
    updated_at = NOW()
WHERE id = $1`

	sqlUpdateCIRunStatus = `
UPDATE ci_configs
SET last_run_status = $2,
    last_run_message = $3,
    updated_at = NOW()
WHERE id = $1`
)

const (
	sqlCreateOperationLog = `
INSERT INTO operation_logs (
    application_id,
    project_id,
    operation_type,
    user_id,
    ci_config_id,
    release_script_id,
    release_script_name,
    release_branch,
    module_name,
    module_path,
    artifact_id,
    artifact_image,
    compose_service_name,
    deploy_remote_dir
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
RETURNING id`

	sqlUpdateOperationLogStatus = `
UPDATE operation_logs
SET status = $2,
    message = $3,
    finished_at = NOW()
WHERE id = $1`

	sqlGetOperationLog = `
SELECT ol.id, ol.application_id, a.name AS application_name,
       ol.project_id, p.name AS project_name,
       ol.user_id, u.username,
       ol.ci_config_id, ol.release_script_id, ol.release_script_name,
       ol.release_branch, ol.module_name, ol.module_path,
       ol.artifact_id, ol.artifact_image, ol.compose_service_name, ol.deploy_remote_dir,
       ol.operation_type, ol.status, ol.message,
       ol.started_at, ol.finished_at
FROM operation_logs ol
JOIN applications a ON ol.application_id = a.id
JOIN projects p ON ol.project_id = p.id
LEFT JOIN users u ON ol.user_id = u.id
WHERE ol.id = $1`
)

const (
	sqlCreateUser = `
INSERT INTO users (username, password_hash)
VALUES ($1, $2)
RETURNING id`

	sqlGetUserByID = `
SELECT id, username, password_hash, created_at, updated_at
FROM users
WHERE id = $1`

	sqlGetUserByUsername = `
SELECT id, username, password_hash, created_at, updated_at
FROM users
WHERE username = $1`
)

const (
	sqlListArtifactRegistries = `
SELECT id, name, provider, registry_url, namespace, repository_names, auth_type, username, credential,
       enabled, last_sync_status, last_sync_message, last_synced_at,
       created_at, updated_at
FROM artifact_registries
ORDER BY name`

	sqlGetArtifactRegistry = `
SELECT id, name, provider, registry_url, namespace, repository_names, auth_type, username, credential,
       enabled, last_sync_status, last_sync_message, last_synced_at,
       created_at, updated_at
FROM artifact_registries
WHERE id = $1`

	sqlCreateArtifactRegistry = `
INSERT INTO artifact_registries (
    name, provider, registry_url, namespace, repository_names, auth_type, username, credential, enabled
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id`

	sqlUpdateArtifactRegistry = `
UPDATE artifact_registries
SET name = $2,
    provider = $3,
    registry_url = $4,
    namespace = $5,
    repository_names = $6,
    auth_type = $7,
    username = $8,
    credential = CASE WHEN $10 THEN credential ELSE $9 END,
    enabled = $11,
    updated_at = NOW()
WHERE id = $1`

	sqlDeleteArtifactRegistry = `
DELETE FROM artifact_registries
WHERE id = $1`

	sqlUpdateArtifactRegistrySyncStatusSuccess = `
UPDATE artifact_registries
SET last_sync_status = $2,
    last_sync_message = $3,
    last_synced_at = NOW(),
    updated_at = NOW()
WHERE id = $1`

	sqlUpdateArtifactRegistrySyncStatus = `
UPDATE artifact_registries
SET last_sync_status = $2,
    last_sync_message = $3,
    updated_at = NOW()
WHERE id = $1`

	sqlGetArtifact = `
SELECT a.id, a.registry_id, ar.name AS registry_name, ar.provider, ar.registry_url, ar.namespace,
       a.repository, a.tag, a.digest, a.media_type, a.size_bytes, a.pushed_at,
       a.remote_status, a.last_synced_at, a.deleted_at, a.created_at, a.updated_at
FROM artifacts a
JOIN artifact_registries ar ON a.registry_id = ar.id
WHERE a.id = $1`

	sqlListArtifactTagsByRepositories = `
SELECT tag
FROM artifacts
WHERE registry_id = $1
  AND repository = ANY($2::text[])
  AND remote_status = 'present'
  AND (tag = $3 OR tag LIKE $3 || '.%')`

	sqlUpsertArtifact = `
INSERT INTO artifacts (
    registry_id, repository, tag, digest, media_type, size_bytes, pushed_at,
    remote_status, last_synced_at, deleted_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, 'present', NOW(), NULL)
ON CONFLICT (registry_id, repository, tag) DO UPDATE SET
    digest = EXCLUDED.digest,
    media_type = EXCLUDED.media_type,
    size_bytes = EXCLUDED.size_bytes,
    pushed_at = EXCLUDED.pushed_at,
    remote_status = 'present',
    last_synced_at = NOW(),
    deleted_at = NULL,
    updated_at = NOW()`

	sqlDeleteAllSyncedArtifacts = `
DELETE FROM artifacts
WHERE registry_id = $1`

	sqlDeleteMissingArtifacts = `
DELETE FROM artifacts
WHERE registry_id = $1
  AND repository || E'\n' || tag <> ALL($2::text[])`

	sqlMarkArtifactDeleted = `
UPDATE artifacts
SET remote_status = 'deleted',
    deleted_at = NOW(),
    updated_at = NOW()
WHERE id = $1`

	sqlCreateArtifactOperationLog = `
INSERT INTO artifact_operation_logs (registry_id, artifact_id, operation_type)
VALUES ($1, $2, $3)
RETURNING id`

	sqlUpdateArtifactOperationLogStatus = `
UPDATE artifact_operation_logs
SET status = $2,
    message = $3,
    finished_at = NOW()
WHERE id = $1`

	sqlGetArtifactOperationLog = `
SELECT id, registry_id, artifact_id, operation_type, status, message, started_at, finished_at
FROM artifact_operation_logs
WHERE id = $1`
)

const (
	sqlListDockerDeployMappings = `
SELECT dm.id, dm.ci_config_id, dm.registry_id, dm.repository,
       dm.compose_service_name, dm.remote_dir, dm.compose_file, dm.description, dm.enabled,
       dm.script_id, dm.script_params, dm.ssh_mode, dm.deploy_ssh,
       dm.last_deploy_status, dm.last_deploy_message, dm.last_deployed_at,
       dm.created_at, dm.updated_at,
       cc.application_id, a.name AS application_name,
       a.project_id, p.name AS project_name,
       cc.release_branch, cc.module_name, cc.module_path,
       ar.name AS registry_name, ar.registry_url
FROM docker_deploy_mappings dm
JOIN ci_configs cc ON dm.ci_config_id = cc.id
JOIN applications a ON cc.application_id = a.id
JOIN projects p ON a.project_id = p.id
JOIN artifact_registries ar ON dm.registry_id = ar.id
ORDER BY p.name, a.name, cc.module_name, dm.compose_service_name`

	sqlGetDockerDeployMapping = `
SELECT dm.id, dm.ci_config_id, dm.registry_id, dm.repository,
       dm.compose_service_name, dm.remote_dir, dm.compose_file, dm.description, dm.enabled,
       dm.script_id, dm.script_params, dm.ssh_mode, dm.deploy_ssh,
       dm.last_deploy_status, dm.last_deploy_message, dm.last_deployed_at,
       dm.created_at, dm.updated_at,
       cc.application_id, a.name AS application_name,
       a.project_id, p.name AS project_name,
       cc.release_branch, cc.module_name, cc.module_path,
       ar.name AS registry_name, ar.registry_url
FROM docker_deploy_mappings dm
JOIN ci_configs cc ON dm.ci_config_id = cc.id
JOIN applications a ON cc.application_id = a.id
JOIN projects p ON a.project_id = p.id
JOIN artifact_registries ar ON dm.registry_id = ar.id
WHERE dm.id = $1`

	sqlCreateDockerDeployMapping = `
INSERT INTO docker_deploy_mappings (
    ci_config_id, registry_id, repository, compose_service_name,
    remote_dir, compose_file, description, enabled, script_id, script_params, ssh_mode, deploy_ssh
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING id`

	sqlUpdateDockerDeployMapping = `
UPDATE docker_deploy_mappings
SET ci_config_id = $2,
    registry_id = $3,
    repository = $4,
    compose_service_name = $5,
    remote_dir = $6,
    compose_file = $7,
    description = $8,
    enabled = $9,
    script_id = $10, script_params = $11, ssh_mode = $12, deploy_ssh = $13,
    updated_at = NOW()
WHERE id = $1`

	sqlDeleteDockerDeployMapping = `
DELETE FROM docker_deploy_mappings
WHERE id = $1`

	sqlUpdateDockerDeployStatusSuccess = `
UPDATE docker_deploy_mappings
SET last_deploy_status = $2,
    last_deploy_message = $3,
    last_deployed_at = NOW(),
    updated_at = NOW()
WHERE id = $1`

	sqlUpdateDockerDeployStatus = `
UPDATE docker_deploy_mappings
SET last_deploy_status = $2,
    last_deploy_message = $3,
    updated_at = NOW()
WHERE id = $1`

	sqlCreateDockerDeployRun = `
INSERT INTO docker_deploy_runs (
    mapping_id,
    operation_log_id,
    artifact_id,
    artifact_image
)
VALUES ($1, $2, $3, $4)
RETURNING id, mapping_id, operation_log_id, artifact_id, artifact_image, status, message, started_at, finished_at`

	sqlUpdateDockerDeployRunStatus = `
UPDATE docker_deploy_runs
SET status = $2,
    message = $3,
    finished_at = NOW()
WHERE id = $1
RETURNING id, mapping_id, operation_log_id, artifact_id, artifact_image, status, message, started_at, finished_at`

	sqlCreateDockerDeployRunLog = `
INSERT INTO docker_deploy_run_logs (run_id, line_no, content)
VALUES ($1, $2, $3)
RETURNING id, run_id, line_no, content, created_at`

	sqlGetLatestDockerDeployRun = `
SELECT id, mapping_id, operation_log_id, artifact_id, artifact_image, status, message, started_at, finished_at
FROM docker_deploy_runs
WHERE mapping_id = $1
ORDER BY started_at DESC, id DESC
LIMIT 1`

	sqlGetDockerDeployRun = `
SELECT id, mapping_id, operation_log_id, artifact_id, artifact_image, status, message, started_at, finished_at
FROM docker_deploy_runs
WHERE id = $1
  AND mapping_id = $2`

	sqlListDockerDeployRunLogs = `
SELECT id, run_id, line_no, content, created_at
FROM docker_deploy_run_logs
WHERE run_id = $1
ORDER BY line_no`

	sqlListArtifactsByRepository = `
SELECT a.id, a.registry_id, ar.name AS registry_name, ar.provider, ar.registry_url, ar.namespace,
       a.repository, a.tag, a.digest, a.media_type, a.size_bytes, a.pushed_at,
       a.remote_status, a.last_synced_at, a.deleted_at, a.created_at, a.updated_at
FROM artifacts a
JOIN artifact_registries ar ON a.registry_id = ar.id
WHERE a.registry_id = $1
  AND a.repository = $2
  AND a.remote_status = 'present'
ORDER BY COALESCE(a.pushed_at, a.last_synced_at, a.created_at) DESC, a.tag DESC`

	sqlListArtifactRepositories = `
SELECT a.registry_id, ar.name AS registry_name, a.repository, COUNT(*) AS tag_count
FROM artifacts a
JOIN artifact_registries ar ON a.registry_id = ar.id
WHERE a.remote_status = 'present'
GROUP BY a.registry_id, ar.name, a.repository
ORDER BY ar.name, a.repository`

	sqlArtifactRepositoryExists = `
SELECT EXISTS (
    SELECT 1
    FROM artifacts
    WHERE registry_id = $1
      AND repository = $2
      AND remote_status = 'present'
)`

	sqlListDockerDeployRepositoriesForArtifactBuild = `
SELECT DISTINCT dm.repository
FROM docker_deploy_mappings dm
JOIN ci_configs cc ON dm.ci_config_id = cc.id
JOIN ci_configs artifact_cc ON artifact_cc.application_id = cc.application_id
WHERE artifact_cc.id = $1
  AND artifact_cc.config_type = 'artifact'
  AND cc.config_type = 'ci'
  AND cc.module_name = artifact_cc.module_name
  AND dm.registry_id = $2
  AND dm.enabled = TRUE
ORDER BY dm.repository`
)

const (
	sqlCreateSession = `
INSERT INTO sessions (token, user_id, expires_at)
VALUES ($1, $2, $3)
RETURNING id`

	sqlGetSessionByToken = `
SELECT id, token, user_id, created_at, expires_at
FROM sessions
WHERE token = $1
  AND expires_at > NOW()`

	sqlDeleteSession = `
DELETE FROM sessions
WHERE token = $1`

	sqlDeleteExpiredSessions = `
DELETE FROM sessions
WHERE expires_at < NOW()`
)

const (
	sqlGetGlobalToolConfig = `
SELECT id, git_path, maven_home_path, maven_user_settings_file,
       maven_local_repository, npm_home_path, npm_user_config_file,
       npm_cache_directory, created_at, updated_at
FROM global_tool_config
WHERE id = 1`

	sqlUpsertGlobalToolConfig = `
INSERT INTO global_tool_config (
    id,
    git_path,
    maven_home_path,
    maven_user_settings_file,
    maven_local_repository,
    npm_home_path,
    npm_user_config_file,
    npm_cache_directory
)
VALUES (1, $1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (id) DO UPDATE SET
    git_path = EXCLUDED.git_path,
    maven_home_path = EXCLUDED.maven_home_path,
    maven_user_settings_file = EXCLUDED.maven_user_settings_file,
    maven_local_repository = EXCLUDED.maven_local_repository,
    npm_home_path = EXCLUDED.npm_home_path,
    npm_user_config_file = EXCLUDED.npm_user_config_file,
    npm_cache_directory = EXCLUDED.npm_cache_directory,
    updated_at = NOW()`

	sqlEnsureGlobalToolConfigTable = `
CREATE TABLE IF NOT EXISTS global_tool_config (
    id                       SMALLINT      PRIMARY KEY DEFAULT 1,
    git_path                 VARCHAR(1024) NOT NULL DEFAULT 'git',
    maven_home_path          VARCHAR(1024) NOT NULL DEFAULT '',
    maven_user_settings_file VARCHAR(1024) NOT NULL DEFAULT '',
    maven_local_repository   VARCHAR(1024) NOT NULL DEFAULT '',
    npm_home_path            VARCHAR(1024) NOT NULL DEFAULT '',
    npm_user_config_file     VARCHAR(1024) NOT NULL DEFAULT '',
    npm_cache_directory      VARCHAR(1024) NOT NULL DEFAULT '',
    created_at               TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT global_tool_config_singleton CHECK (id = 1)
)`

	sqlEnsureGlobalToolConfigRow = `
INSERT INTO global_tool_config (
    id,
    git_path,
    maven_home_path,
    maven_user_settings_file,
    maven_local_repository,
    npm_home_path,
    npm_user_config_file,
    npm_cache_directory
)
VALUES (1, 'git', '', '', '', '', '', '')
ON CONFLICT (id) DO NOTHING`

	sqlGlobalToolConfigColumnExists = `
SELECT EXISTS (
    SELECT 1
    FROM information_schema.columns
    WHERE table_name = 'global_tool_config'
      AND column_name = $1
)`
)

func buildListOperationLogsSQL(projectID int64, applicationID int64, limit int, offset int) (string, []any) {
	var q strings.Builder
	args := make([]any, 0, 4)
	n := 1

	q.WriteString(`
SELECT ol.id, ol.application_id, a.name AS application_name,
       ol.project_id, p.name AS project_name,
       ol.user_id, u.username,
       ol.ci_config_id, ol.release_script_id, ol.release_script_name,
       ol.release_branch, ol.module_name, ol.module_path,
       ol.artifact_id, ol.artifact_image, ol.compose_service_name, ol.deploy_remote_dir,
       ol.operation_type, ol.status, ol.message,
       ol.started_at, ol.finished_at
FROM operation_logs ol
JOIN applications a ON ol.application_id = a.id
JOIN projects p ON ol.project_id = p.id
LEFT JOIN users u ON ol.user_id = u.id`)

	appendOperationLogFilters(&q, &args, &n, "ol.project_id", "ol.application_id", projectID, applicationID)

	q.WriteString(`
ORDER BY ol.started_at DESC`)
	fmt.Fprintf(&q, `
LIMIT $%d OFFSET $%d`, n, n+1)
	args = append(args, limit, offset)

	return q.String(), args
}

func buildCountOperationLogsSQL(projectID int64, applicationID int64) (string, []any) {
	var q strings.Builder
	args := make([]any, 0, 2)
	n := 1

	q.WriteString(`
SELECT COUNT(*)
FROM operation_logs`)

	appendOperationLogFilters(&q, &args, &n, "project_id", "application_id", projectID, applicationID)

	return q.String(), args
}

func buildListArtifactsSQL(registryID int64, status string) (string, []any) {
	var q strings.Builder
	args := make([]any, 0, 2)
	n := 1

	q.WriteString(`
SELECT a.id, a.registry_id, ar.name AS registry_name, ar.provider, ar.registry_url, ar.namespace,
       a.repository, a.tag, a.digest, a.media_type, a.size_bytes, a.pushed_at,
       a.remote_status, a.last_synced_at, a.deleted_at, a.created_at, a.updated_at
FROM artifacts a
JOIN artifact_registries ar ON a.registry_id = ar.id`)

	if registryID > 0 {
		fmt.Fprintf(&q, `
WHERE a.registry_id = $%d`, n)
		args = append(args, registryID)
		n++
	}
	status = strings.TrimSpace(status)
	if status != "" {
		if n == 1 {
			q.WriteString(`
WHERE`)
		} else {
			q.WriteString(`
  AND`)
		}
		fmt.Fprintf(&q, " a.remote_status = $%d", n)
		args = append(args, status)
	}

	q.WriteString(`
ORDER BY COALESCE(a.pushed_at, a.last_synced_at, a.created_at) DESC,
         ar.name, a.repository, a.tag`)
	return q.String(), args
}

func appendOperationLogFilters(q *strings.Builder, args *[]any, n *int, projectColumn string, applicationColumn string, projectID int64, applicationID int64) {
	if projectID > 0 {
		fmt.Fprintf(q, `
WHERE %s = $%d`, projectColumn, *n)
		*args = append(*args, projectID)
		*n++
	}
	if applicationID > 0 {
		if *n == 1 {
			q.WriteString(`
WHERE`)
		} else {
			q.WriteString(`
  AND`)
		}
		fmt.Fprintf(q, " %s = $%d", applicationColumn, *n)
		*args = append(*args, applicationID)
		*n++
	}
}

func sqlAddGlobalToolConfigColumn(name string, definition string) string {
	return fmt.Sprintf("ALTER TABLE global_tool_config ADD COLUMN %s %s", name, definition)
}
