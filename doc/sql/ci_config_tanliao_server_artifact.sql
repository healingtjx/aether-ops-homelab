-- tanliao-server 项目及镜像打包配置；可重复执行，不创建部署配置。
BEGIN;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM artifact_registries WHERE name = 'tanliao AWS ECR' AND provider = 'aws_ecr' AND enabled) THEN
        RAISE EXCEPTION '缺少已启用的 tanliao AWS ECR 仓库';
    END IF;
END $$;

INSERT INTO projects (name, description)
VALUES ('tanliao-server', '探聊服务端，独立项目，包含 tio-site-all 和 pc-html 镜像打包')
ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description, updated_at = NOW();

INSERT INTO applications (project_id, name, description, git_url, default_branch)
SELECT id, 'tanliao-server', '探聊后端与 PC 前端；本地参考代码 /Users/tjx/IdeaProjects/tanliao-server',
       'https://github.com/healingtjx/tanliao-server.git', 'master'
FROM projects WHERE name = 'tanliao-server'
ON CONFLICT (project_id, name) DO UPDATE
SET description = EXCLUDED.description, git_url = EXCLUDED.git_url,
    default_branch = EXCLUDED.default_branch, updated_at = NOW();

INSERT INTO release_scripts (name, description, content, default_variables)
VALUES ('tanliao-server-ECR镜像打包',
        '调用项目 devops/build_push_prod.sh，Java 8 / Vue 构建并推送 linux/amd64 ECR 镜像；兼容 Linux sed',
        $tanliao_server_script$#!/usr/bin/env bash
# 平台注入模块和版本；项目仓库负责 Maven/npm 构建、ECR 登录和镜像推送。
set -Eeuo pipefail

: "${MODULE_NAME:?缺少 MODULE_NAME}"
case "$MODULE_NAME" in
  tio-site-all) module_dir=all ;;
  pc-html) module_dir=pc-html ;;
  *) echo "不支持的模块: $MODULE_NAME" >&2; exit 1 ;;
esac
IMAGE_TAG="${IMAGE_TAG:-${ARTIFACT_TAG:-}}"
: "${IMAGE_TAG:?请填写 IMAGE_TAG 镜像版本}"
[[ "$IMAGE_TAG" =~ ^[a-zA-Z0-9_][a-zA-Z0-9_.-]{0,127}$ ]] || {
  echo '镜像版本格式不正确' >&2; exit 1;
}
test -f devops/build_push_prod.sh
test -f "$module_dir/Dockerfile"
# 当前项目的登录命令固定使用此 ECR，禁止登录与推送目标不一致。
export DOCKER_REGISTRY="${DOCKER_REGISTRY:-319966842764.dkr.ecr.ap-northeast-1.amazonaws.com}"
[[ "$DOCKER_REGISTRY" == 319966842764.dkr.ecr.ap-northeast-1.amazonaws.com ]] || {
  echo '当前项目脚本仅支持 tanliao AWS ECR' >&2; exit 1;
}
if [[ -z "${AWS_PROFILE:-}" ]]; then unset AWS_PROFILE; fi
for cmd in aws docker; do command -v "$cmd" >/dev/null; done
docker buildx version

if [[ "$MODULE_NAME" == tio-site-all ]]; then
  export JAVA_HOME="${JDK_HOME:-${JAVA_HOME:-/usr/lib/jvm/java-8-openjdk-amd64}}"
  test -x "$JAVA_HOME/bin/java"
  export PATH="$JAVA_HOME/bin:$PATH"
  "$JAVA_HOME/bin/java" -version 2>&1 | grep -q 'version "1.8.' || {
    echo '后端要求 Java 8，请设置 JDK_HOME' >&2; exit 1;
  }
  command -v mvn >/dev/null
  # 仅修改平台拉取的工作副本，下次拉取会重置；不改开发者本机代码。
  # sed -i.bak 同时兼容 GNU/BSD；子脚本自身也必须在 Maven 失败时退出。
  python3 - <<'PY'
from pathlib import Path

path = Path('all/package-linux-prod.sh')
if path.is_file():
    content = path.read_text().replace('sed -i .bak ', 'sed -i.bak ')
    lines = content.splitlines(keepends=True)
    offset = 1 if lines and lines[0].startswith('#!') else 0
    lines.insert(offset, 'set -Ee -o pipefail\n')
    path.write_text(''.join(lines))
PY
else
  node_major="$(node -p 'process.versions.node.split(".")[0]')"
  if (( node_major >= 17 )); then
    export NODE_OPTIONS="${NODE_OPTIONS:-} --openssl-legacy-provider"
  fi
fi

echo "调用项目打包脚本: devops/build_push_prod.sh $MODULE_NAME $IMAGE_TAG"
exec bash -e -o pipefail devops/build_push_prod.sh "$MODULE_NAME" "$IMAGE_TAG"
$tanliao_server_script$,
        '{"JDK_HOME":"/usr/lib/jvm/java-8-openjdk-amd64","IMAGE_TAG":"","AWS_REGION":"ap-northeast-1","DOCKER_REGISTRY":"319966842764.dkr.ecr.ap-northeast-1.amazonaws.com"}'::jsonb)
ON CONFLICT (name) DO UPDATE
SET description = EXCLUDED.description, content = EXCLUDED.content,
    default_variables = EXCLUDED.default_variables, updated_at = NOW();

-- 追加同步范围，保留已有 tio-mg-* 镜像和原有顺序。
UPDATE artifact_registries ar
SET repository_names = (
    SELECT string_agg(repository, ',' ORDER BY position)
    FROM (
        SELECT btrim(value) AS repository, min(ordinality) AS position
        FROM unnest(string_to_array(COALESCE(ar.repository_names, '') || ',tio-site-all,tio-site-pc-html', ','))
             WITH ORDINALITY AS items(value, ordinality)
        WHERE btrim(value) <> ''
        GROUP BY btrim(value)
    ) repositories
), updated_at = NOW()
WHERE name = 'tanliao AWS ECR';

INSERT INTO ci_configs
(application_id, release_script_id, release_branch, module_name, module_path,
 config_type, build_mode, artifact_registry_id, artifact_release_script_id, variables)
SELECT a.id, rs.id, 'master', modules.name, modules.path,
       'artifact', 'docker', ar.id, rs.id,
       jsonb_build_object('IMAGE_REPOSITORY', modules.repository)
FROM applications a
JOIN projects p ON p.id = a.project_id
CROSS JOIN release_scripts rs
CROSS JOIN artifact_registries ar
CROSS JOIN (VALUES ('tio-site-all', 'all', 'tio-site-all'),
                   ('pc-html', 'pc-html', 'tio-site-pc-html')) AS modules(name, path, repository)
WHERE p.name = 'tanliao-server' AND a.name = 'tanliao-server'
  AND rs.name = 'tanliao-server-ECR镜像打包' AND ar.name = 'tanliao AWS ECR'
ON CONFLICT (application_id, release_script_id, module_name) DO UPDATE
SET release_branch = EXCLUDED.release_branch, module_path = EXCLUDED.module_path,
    config_type = EXCLUDED.config_type, build_mode = EXCLUDED.build_mode,
    artifact_registry_id = EXCLUDED.artifact_registry_id,
    artifact_release_script_id = EXCLUDED.artifact_release_script_id,
    variables = COALESCE(ci_configs.variables, '{}'::jsonb) || EXCLUDED.variables,
    updated_at = NOW();

COMMIT;
