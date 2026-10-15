INSERT INTO release_scripts (name, description, content, default_variables)
VALUES (
    '贝斯-bms-前端-新增产物',
    '调用 bs-code/bs-web-pc/build_push_web_prod.sh，使用 prod 环境打包并推送 BMS 前端 Docker 镜像',
    $script$
#!/usr/bin/env bash
set -Eeuo pipefail

BUILD_PUSH_WEB_SCRIPT="${BUILD_PUSH_WEB_SCRIPT:-/Users/tjx/code/bs-code/bs-web-pc/build_push_web_prod.sh}"

IMAGE_TAG="${IMAGE_TAG:-${ARTIFACT_TAG:-${WEB_VERSION:-${RELEASE_BRANCH:-}}}}"
if [ -z "${IMAGE_TAG}" ]; then
    echo "缺少前端镜像标签：请设置 IMAGE_TAG、ARTIFACT_TAG、WEB_VERSION，或在配置中设置发布分支 RELEASE_BRANCH"
    exit 1
fi

if [ ! -f "${BUILD_PUSH_WEB_SCRIPT}" ]; then
    echo "未找到前端构建推送脚本: ${BUILD_PUSH_WEB_SCRIPT}"
    exit 1
fi

echo "开始新增前端产物: script=${BUILD_PUSH_WEB_SCRIPT}, tag=${IMAGE_TAG}"
bash "${BUILD_PUSH_WEB_SCRIPT}" "${IMAGE_TAG}"
$script$,
    '{"BUILD_PUSH_WEB_SCRIPT": "/Users/tjx/code/bs-code/bs-web-pc/build_push_web_prod.sh"}'::jsonb
)
ON CONFLICT (name)
DO UPDATE SET
    description = EXCLUDED.description,
    content = EXCLUDED.content,
    default_variables = EXCLUDED.default_variables,
    updated_at = NOW();
