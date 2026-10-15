WITH target_ci AS (
    SELECT cc.id AS ci_config_id
    FROM ci_configs cc
    JOIN applications a ON a.id = cc.application_id
    JOIN projects p ON p.id = a.project_id
    WHERE p.name = 'beisi'
      AND a.name = 'bs-code'
      AND cc.config_type = 'ci'
      AND cc.module_name = 'forkliftems-mcp'
),
target_registry AS (
    SELECT id AS registry_id
    FROM artifact_registries
    WHERE name = '新加坡-阿里云'
    ORDER BY id
    LIMIT 1
)
INSERT INTO docker_deploy_mappings (
    ci_config_id,
    registry_id,
    repository,
    compose_service_name,
    remote_dir,
    compose_file,
    description,
    enabled
)
SELECT
    target_ci.ci_config_id,
    target_registry.registry_id,
    'bs_code/bs-forkliftems-mcp',
    'bmser-forkliftems-mcp',
    '/opt/docker-compsoe',
    'docker-compose.yml',
    '叉车 MCP 服务',
    TRUE
FROM target_ci
CROSS JOIN target_registry
ON CONFLICT (ci_config_id, registry_id, repository, compose_service_name, remote_dir, compose_file)
DO UPDATE SET
    description = EXCLUDED.description,
    enabled = EXCLUDED.enabled,
    updated_at = NOW();
