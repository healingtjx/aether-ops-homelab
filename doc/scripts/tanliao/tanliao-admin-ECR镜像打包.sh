#!/usr/bin/env bash
# 平台负责拉取代码和注入变量；编译、登录和镜像推送统一由项目脚本负责。
set -Eeuo pipefail

: "${MODULE_NAME:?缺少 MODULE_NAME}"
case "$MODULE_NAME" in
  tio-mg-all|pc-html) ;;
  *) echo "不支持的模块: $MODULE_NAME" >&2; exit 1 ;;
esac
IMAGE_TAG="${IMAGE_TAG:-${ARTIFACT_TAG:-}}"
: "${IMAGE_TAG:?请填写 IMAGE_TAG 镜像版本}"
[[ "$IMAGE_TAG" =~ ^[a-zA-Z0-9_][a-zA-Z0-9_.-]{0,127}$ ]] || {
  echo "镜像版本格式不正确" >&2; exit 1;
}
test -f devops/build_push_prod.sh || {
  echo "项目缺少 devops/build_push_prod.sh，请检查发布分支" >&2; exit 1;
}
# 避免空 profile 被 AWS CLI 识别为一个不存在的配置名称。
if [[ -z "${AWS_PROFILE:-}" ]]; then unset AWS_PROFILE; fi
if [[ "$MODULE_NAME" == tio-mg-all ]]; then
  export JAVA_HOME="${JDK_HOME:-${JAVA_HOME:-/usr/lib/jvm/java-8-openjdk-amd64}}"
  test -x "$JAVA_HOME/bin/java"
  export PATH="$JAVA_HOME/bin:$PATH"
  "$JAVA_HOME/bin/java" -version 2>&1 | grep -q 'version "1.8.' || {
    echo "后端要求 Java 8，请设置 JDK_HOME" >&2; exit 1;
  }
else
  node_major="$(node -p 'process.versions.node.split(".")[0]')"
  if (( node_major >= 17 )); then
    export NODE_OPTIONS="${NODE_OPTIONS:-} --openssl-legacy-provider"
  fi
fi
echo "调用项目打包脚本: devops/build_push_prod.sh $MODULE_NAME $IMAGE_TAG"
exec bash devops/build_push_prod.sh "$MODULE_NAME" "$IMAGE_TAG"
