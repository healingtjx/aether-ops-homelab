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
),
modules(module_name, module_path, project_name, remote_jar_name, build_dir) AS (
    VALUES
        ('forkliftems-message-proxy', 'bs-forkliftems-message-proxy-master', 'forkliftems-message', 'forkliftems-message-proxy.jar', 'bs-forkliftems-message-proxy-master'),
        ('houseems-message-proxy', 'bs-houseems-message-proxy-master', 'houseems-message', 'houseems-message-proxy.jar', 'bs-houseems-message-proxy-master'),
        ('forkliftems-bms', 'bs_forkliftems-master/forkliftems-service/forkliftems-bms', 'forkliftems', 'blade-forkliftems.jar', 'bs_forkliftems-master/forkliftems-service/forkliftems-bms'),
        ('forkliftems-mcp', 'bs_forkliftems-master/forkliftems-service/forkliftems-mcp', 'forkliftems-mcp', 'forkliftems-mcp.jar', 'bs_forkliftems-master/forkliftems-service/forkliftems-mcp'),
        ('houseems-bms', 'bs_houseems-master/houseems-service/houseems-bms', 'houseems', 'blade-houseems.jar', 'bs_houseems-master/houseems-service/houseems-bms'),
        ('houseems-mcp', 'bs_houseems-master/houseems-service/houseems-mcp', 'houseems-mcp', 'houseems-mcp.jar', 'bs_houseems-master/houseems-service/houseems-mcp'),
        ('auth', 'bs_portal-master/blade-auth', 'auth', 'blade-auth.jar', 'bs_portal-master/blade-auth'),
        ('gateway', 'bs_portal-master/blade-gateway', 'gateway', 'blade-gateway.jar', 'bs_portal-master/blade-gateway'),
        ('log', 'bs_portal-master/blade-ops/blade-log', 'log', 'blade-log.jar', 'bs_portal-master/blade-ops/blade-log'),
        ('resource', 'bs_portal-master/blade-ops/blade-resource', 'resource', 'blade-resource.jar', 'bs_portal-master/blade-ops/blade-resource'),
        ('agent', 'bs_portal-master/blade-agent', 'agent', 'blade-agent.jar', 'bs_portal-master/blade-agent'),
        ('system', 'bs_portal-master/blade-service/blade-system', 'system', 'blade-system.jar', 'bs_portal-master/blade-service/blade-system'),
        ('user', 'bs_portal-master/blade-service/blade-user', 'user', 'blade-user.jar', 'bs_portal-master/blade-service/blade-user')
)
INSERT INTO ci_configs (
    application_id,
    release_script_id,
    release_branch,
    module_name,
    module_path,
    variables
)
SELECT
    target_app.application_id,
    target_script.release_script_id,
    target_app.default_branch,
    modules.module_name,
    modules.module_path,
    jsonb_build_object(
        'BUILD_DIR', modules.build_dir,
        'PROJECT_NAME', modules.project_name,
        'REMOTE_JAR_NAME', modules.remote_jar_name
    )
FROM target_app
CROSS JOIN target_script
CROSS JOIN modules
ON CONFLICT (application_id, release_script_id, module_name)
DO UPDATE SET
    release_branch = EXCLUDED.release_branch,
    module_path = EXCLUDED.module_path,
    variables = EXCLUDED.variables,
    updated_at = NOW();
