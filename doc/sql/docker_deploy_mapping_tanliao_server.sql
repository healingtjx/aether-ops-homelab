-- tanliao-server 后端生产发布：复用管理端已有跳板机连接，不执行发布。
BEGIN;

DO $$
BEGIN
    IF (SELECT count(*)
        FROM docker_deploy_mappings dm
        JOIN ci_configs cc ON cc.id = dm.ci_config_id
        JOIN applications a ON a.id = cc.application_id
        WHERE a.name = 'tanliao-admin' AND cc.module_name = 'tio-mg-all'
          AND dm.script_id = 'bastion-deploy.json'
          AND dm.ssh_mode = 'independent' AND dm.enabled) <> 1 THEN
        RAISE EXCEPTION '需要唯一且启用的 tanliao-admin 后端跳板机发布映射';
    END IF;
    IF (SELECT count(*)
        FROM ci_configs cc
        JOIN applications a ON a.id = cc.application_id
        JOIN projects p ON p.id = a.project_id
        WHERE p.name = 'tanliao-server' AND a.name = 'tanliao-server'
          AND cc.config_type = 'ci' AND cc.module_name = 'tio-site-all') <> 1 THEN
        RAISE EXCEPTION '需要唯一的 tanliao-server / tio-site-all CI 配置';
    END IF;
END $$;

INSERT INTO docker_deploy_mappings
    (ci_config_id, registry_id, repository, compose_service_name,
     remote_dir, compose_file, description, enabled,
     script_id, script_params, ssh_mode, deploy_ssh)
SELECT cc.id, reference.registry_id, 'tio-site-all', 'tio-site-all',
       '/opt/tanliao-serve', 'docker-compose.yaml',
       'tanliao-server 生产后端；通过现有跳板机发布到 160.191.53.106:/opt/tanliao-serve', TRUE,
       reference.script_id,
       reference.script_params || '{"DEPLOY_PROJECT":"tanliao-server","DEPLOY_ENV":"prod"}'::jsonb,
       reference.ssh_mode, reference.deploy_ssh
FROM ci_configs cc
JOIN applications a ON a.id = cc.application_id
JOIN projects p ON p.id = a.project_id
CROSS JOIN (
    SELECT dm.*
    FROM docker_deploy_mappings dm
    JOIN ci_configs source_cc ON source_cc.id = dm.ci_config_id
    JOIN applications source_app ON source_app.id = source_cc.application_id
    WHERE source_app.name = 'tanliao-admin' AND source_cc.module_name = 'tio-mg-all'
      AND dm.script_id = 'bastion-deploy.json'
      AND dm.ssh_mode = 'independent' AND dm.enabled
) reference
WHERE p.name = 'tanliao-server' AND a.name = 'tanliao-server'
  AND cc.config_type = 'ci' AND cc.module_name = 'tio-site-all'
ON CONFLICT (ci_config_id, registry_id, repository, compose_service_name, remote_dir, compose_file)
DO UPDATE SET description = EXCLUDED.description, enabled = EXCLUDED.enabled,
              script_id = EXCLUDED.script_id, script_params = EXCLUDED.script_params,
              ssh_mode = EXCLUDED.ssh_mode, deploy_ssh = EXCLUDED.deploy_ssh,
              updated_at = NOW()
RETURNING id, ci_config_id, repository, compose_service_name, script_id, script_params, enabled;

COMMIT;
