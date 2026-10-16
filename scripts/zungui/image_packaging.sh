#!/usr/bin/env bash
set -Eeuo pipefail

# 粘贴到 Aether Ops 发布脚本中；平台负责拉取项目并注入以下变量。
: "${WORK_DIR:?缺少平台变量 WORK_DIR}"
: "${MODULE_NAME:?缺少平台变量 MODULE_NAME}"
IMAGE_TAG="${IMAGE_TAG:-${ARTIFACT_TAG:-${RELEASE_BRANCH:-}}}"
: "${IMAGE_TAG:?请设置 IMAGE_TAG（例如 26.1016.0）或 RELEASE_BRANCH}"

JDK_HOME="${JDK_HOME:-${JAVA_HOME:-/usr/lib/jvm/java-8-openjdk-amd64}}"

cd "$WORK_DIR"
BUILD_PUSH_SCRIPT="./devops/docker/build_push_prod.sh"
if [[ ! -f "$BUILD_PUSH_SCRIPT" ]]; then
    echo "未找到项目打包脚本: $PWD/$BUILD_PUSH_SCRIPT" >&2
    exit 1
fi

printf '开始打包: module=%s, tag=%s\n' "$MODULE_NAME" "$IMAGE_TAG"
exec bash "$BUILD_PUSH_SCRIPT" --jdk-home "$JDK_HOME" "$MODULE_NAME" "$IMAGE_TAG"
