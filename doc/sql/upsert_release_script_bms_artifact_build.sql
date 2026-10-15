INSERT INTO release_scripts (name, description, content, default_variables)
VALUES (
    '贝斯-bms-服务器-新增产物',
    '调用当前工作目录 bs-code/devops/build_push_prod.sh，使用 prod 环境打包并推送 BMS 服务端 Docker 镜像',
    $script$
#!/usr/bin/env bash
set -Eeuo pipefail

BUILD_PUSH_SCRIPT="${BUILD_PUSH_SCRIPT:-${WORK_DIR}/devops/build_push_prod.sh}"
BUILD_PUSH_ENV="prod"

IMAGE_TAG="${IMAGE_TAG:-${ARTIFACT_TAG:-${RELEASE_BRANCH:-}}}"
if [ -z "${IMAGE_TAG}" ]; then
    echo "缺少镜像标签：请设置 IMAGE_TAG，或在配置中设置发布分支 RELEASE_BRANCH"
    exit 1
fi

if [ ! -f "${BUILD_PUSH_SCRIPT}" ]; then
    echo "未找到构建推送脚本: ${BUILD_PUSH_SCRIPT}"
    exit 1
fi

normalize_module() {
    case "$1" in
        bs-forkliftems|forkliftems|forkliftems-bms)
            printf '%s\n' "bs-forkliftems"
            ;;
        bs-forkliftems-mcp|forkliftems-mcp)
            printf '%s\n' "bs-forkliftems-mcp"
            ;;
        bs-houseems|houseems|houseems-bms)
            printf '%s\n' "bs-houseems"
            ;;
        bs-houseems-mcp|houseems-mcp)
            printf '%s\n' "bs-houseems-mcp"
            ;;
        bs-blade-system|system|blade-system)
            printf '%s\n' "bs-blade-system"
            ;;
        bs-blade-gateway|gateway|blade-gateway)
            printf '%s\n' "bs-blade-gateway"
            ;;
        bs-blade-user|user|blade-user)
            printf '%s\n' "bs-blade-user"
            ;;
        bs-blade-log|log|blade-log)
            printf '%s\n' "bs-blade-log"
            ;;
        bs-blade-auth|auth|blade-auth)
            printf '%s\n' "bs-blade-auth"
            ;;
        bs-blade-resource|resource|blade-resource)
            printf '%s\n' "bs-blade-resource"
            ;;
        bs-blade-agent|agent|blade-agent)
            printf '%s\n' "bs-blade-agent"
            ;;
        bs-blade-mqtt|mqtt|blade-mqtt)
            printf '%s\n' "bs-blade-mqtt"
            ;;
        bs-forkliftems-message-proxy|forkliftems-message-proxy)
            printf '%s\n' "bs-forkliftems-message-proxy"
            ;;
        bs-houseems-message-proxy|houseems-message-proxy)
            printf '%s\n' "bs-houseems-message-proxy"
            ;;
        *)
            echo "不支持的构建模块: $1" >&2
            echo "请设置 BUILD_PUSH_MODULE 或 BUILD_PUSH_MODULES 为 build_push_prod.sh 支持的模块名" >&2
            return 1
            ;;
    esac
}

run_build_push() {
    local module="$1"

    echo "开始新增产物: module=${module}, env=${BUILD_PUSH_ENV}, tag=${IMAGE_TAG}"
    bash "${BUILD_PUSH_SCRIPT}" --env "${BUILD_PUSH_ENV}" "${module}" "${IMAGE_TAG}"
}

if [ "${BUILD_PUSH_ALL:-}" = "true" ]; then
    echo "BUILD_PUSH_ALL=true，将执行 build_push_prod.sh 全量构建并推送"
    bash "${BUILD_PUSH_SCRIPT}" --env "${BUILD_PUSH_ENV}" "${IMAGE_TAG}"
    exit $?
fi

MODULES_RAW="${BUILD_PUSH_MODULES:-${BUILD_PUSH_MODULE:-${MODULE_NAME:-}}}"
if [ -z "${MODULES_RAW}" ]; then
    echo "缺少构建模块：请设置 BUILD_PUSH_MODULES、BUILD_PUSH_MODULE，或确保 MODULE_NAME 有值"
    exit 1
fi

MODULES_RAW="${MODULES_RAW//,/ }"
for raw_module in ${MODULES_RAW}; do
    module="$(normalize_module "${raw_module}")"
    run_build_push "${module}"
done
$script$,
    '{}'::jsonb
)
ON CONFLICT (name)
DO UPDATE SET
    description = EXCLUDED.description,
    content = EXCLUDED.content,
    default_variables = EXCLUDED.default_variables,
    updated_at = NOW();
