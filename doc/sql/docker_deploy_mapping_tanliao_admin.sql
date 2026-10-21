BEGIN;
-- 独立发布 SSH 配置，不使用测试环境 REMOTE_* 或全局 SSH 凭据。
UPDATE ci_configs cc
SET variables = COALESCE(cc.variables, '{}'::jsonb) || '{"DEPLOY_SSH_HOST":"160.191.53.106","DEPLOY_SSH_USER":"root","DEPLOY_SSH_PORT":"22"}'::jsonb,
    updated_at = NOW()
FROM applications a JOIN projects p ON p.id = a.project_id
WHERE cc.application_id = a.id AND p.name = 'tanliao' AND a.name = 'tanliao-admin'
  AND cc.config_type = 'ci' AND cc.module_name IN ('tio-mg-all', 'pc-html');

INSERT INTO docker_deploy_mappings
(ci_config_id, registry_id, repository, compose_service_name, remote_dir, compose_file, description, enabled)
SELECT cc.id, ar.id,
       CASE cc.module_name WHEN 'pc-html' THEN 'tio-mg-pc-html' ELSE 'tio-mg-all' END,
       cc.module_name, '/opt/tanliao-admin', 'docker-compose.yaml',
       'tanliao 正式环境 160.191.53.106；配置独立 DEPLOY_SSH_* 凭据后启用', FALSE
FROM ci_configs cc
JOIN applications a ON a.id = cc.application_id
JOIN projects p ON p.id = a.project_id
JOIN artifact_registries ar ON ar.name = 'tanliao AWS ECR'
WHERE p.name = 'tanliao' AND a.name = 'tanliao-admin' AND cc.config_type = 'ci'
  AND cc.module_name IN ('tio-mg-all', 'pc-html')
ON CONFLICT (ci_config_id, registry_id, repository, compose_service_name, remote_dir, compose_file)
DO UPDATE SET description = EXCLUDED.description, updated_at = NOW();
COMMIT;
