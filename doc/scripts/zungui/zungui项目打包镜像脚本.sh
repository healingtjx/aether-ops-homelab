#!/usr/bin/env bash
set -Eeuo pipefail

# 粘贴到 Aether Ops 发布脚本中；平台负责拉取项目并注入以下变量。
: "${WORK_DIR:?缺少平台变量 WORK_DIR}"
: "${MODULE_NAME:?缺少平台变量 MODULE_NAME}"
IMAGE_TAG="${IMAGE_TAG:-${ARTIFACT_TAG:-${RELEASE_BRANCH:-}}}"
: "${IMAGE_TAG:?请设置 IMAGE_TAG（例如 26.1016.0）或 RELEASE_BRANCH}"

JDK_HOME="${JDK_HOME:-${JAVA_HOME:-/usr/lib/jvm/java-8-openjdk-amd64}}"
# 默认使用平台 PATH 中的 Maven 包装脚本，继承全局 settings 和本地仓库配置。
# 如需覆盖，可通过打包运行变量传入以下三个参数。
MAVEN_BIN="${MAVEN_BIN:-mvn}"
MAVEN_SETTINGS="${MAVEN_SETTINGS:-}"
MAVEN_LOCAL_REPOSITORY="${MAVEN_LOCAL_REPOSITORY:-}"

cd "$WORK_DIR"
BUILD_PUSH_SCRIPT="./devops/docker/build_push_prod.sh"
if [[ ! -f "$BUILD_PUSH_SCRIPT" ]]; then
    echo "未找到项目打包脚本: $PWD/$BUILD_PUSH_SCRIPT" >&2
    exit 1
fi

printf '开始打包: module=%s, tag=%s\n' "$MODULE_NAME" "$IMAGE_TAG"
exec bash "$BUILD_PUSH_SCRIPT" \
    --jdk-home "$JDK_HOME" \
    --maven-bin "$MAVEN_BIN" \
    --maven-settings="$MAVEN_SETTINGS" \
    --maven-repo="$MAVEN_LOCAL_REPOSITORY" \
    "$MODULE_NAME" "$IMAGE_TAG"
