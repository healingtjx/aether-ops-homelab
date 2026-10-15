WITH source_ci AS (
    SELECT cc.application_id, cc.release_branch, cc.module_name, cc.module_path, cc.variables
    FROM ci_configs cc
    JOIN applications a ON a.id = cc.application_id
    JOIN projects p ON p.id = a.project_id
    JOIN release_scripts rs ON rs.id = cc.release_script_id
    WHERE p.name = 'beisi'
      AND a.name = 'bs-code'
      AND rs.name = '贝斯-bms-服务端'
      AND cc.config_type = 'ci'
      AND cc.module_name = 'agent'
),
artifact_script AS (
    SELECT id AS release_script_id
    FROM release_scripts
    WHERE name = '贝斯-bms-服务器-新增产物'
),
artifact_registry AS (
    SELECT id AS artifact_registry_id
    FROM artifact_registries
    WHERE name = '新加坡-阿里云'
    ORDER BY id
    LIMIT 1
)
INSERT INTO ci_configs (
    config_type,
    application_id,
    release_script_id,
    artifact_registry_id,
    artifact_release_script_id,
    release_branch,
    module_name,
    module_path,
    variables
)
SELECT
    'artifact',
    source_ci.application_id,
    artifact_script.release_script_id,
    artifact_registry.artifact_registry_id,
    artifact_script.release_script_id,
    source_ci.release_branch,
    source_ci.module_name,
    source_ci.module_path,
    source_ci.variables
FROM source_ci
CROSS JOIN artifact_script
CROSS JOIN artifact_registry
ON CONFLICT (application_id, release_script_id, module_name)
DO UPDATE SET
    config_type = EXCLUDED.config_type,
    artifact_registry_id = EXCLUDED.artifact_registry_id,
    artifact_release_script_id = EXCLUDED.artifact_release_script_id,
    release_branch = EXCLUDED.release_branch,
    module_path = EXCLUDED.module_path,
    variables = EXCLUDED.variables,
    updated_at = NOW();
