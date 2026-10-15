WITH target_app AS (
    SELECT a.id AS application_id, a.default_branch
    FROM applications a
    JOIN projects p ON p.id = a.project_id
    WHERE p.name = 'beisi'
      AND a.name = 'bs-code'
),
target_script AS (
    SELECT id AS release_script_id
    FROM release_scripts
    WHERE name = '贝斯-bms-服务端'
)
INSERT INTO ci_configs (
    config_type,
    application_id,
    release_script_id,
    release_branch,
    module_name,
    module_path,
    variables
)
SELECT
    'ci',
    target_app.application_id,
    target_script.release_script_id,
    target_app.default_branch,
    'houseems-mcp',
    'bs_houseems-master/houseems-service/houseems-mcp',
    jsonb_build_object(
        'BUILD_DIR', 'bs_houseems-master/houseems-service/houseems-mcp',
        'PROJECT_NAME', 'houseems-mcp',
        'REMOTE_JAR_NAME', 'houseems-mcp.jar'
    )
FROM target_app
CROSS JOIN target_script
ON CONFLICT (application_id, release_script_id, module_name)
DO UPDATE SET
    config_type = EXCLUDED.config_type,
    release_branch = EXCLUDED.release_branch,
    module_path = EXCLUDED.module_path,
    variables = EXCLUDED.variables,
    updated_at = NOW();
