#!/usr/bin/env bash
# Aether Ops 在仓库根目录执行；CI 只编译，打包配置设置 BUILD_PUSH=true。
set -Eeuo pipefail

: "${MODULE_NAME:?缺少 MODULE_NAME}"
case "$MODULE_NAME" in
  tio-mg-all) module_dir=all; image_name=tio-mg-all ;;
  pc-html) module_dir=pc-html; image_name=tio-mg-pc-html ;;
  *) echo "不支持的模块: $MODULE_NAME" >&2; exit 1 ;;
esac
# 平台已将当前目录设为仓库根目录，避免再次拼接相对 WORK_DIR。
test -f parent/pom.xml
test -f "$module_dir/Dockerfile"
BUILD_PUSH="${BUILD_PUSH:-false}"
if [[ "$BUILD_PUSH" != false && "$BUILD_PUSH" != true ]]; then
  echo "BUILD_PUSH 必须为 true 或 false" >&2; exit 1
fi
if [[ "$BUILD_PUSH" == true ]]; then
  IMAGE_TAG="${IMAGE_TAG:-${ARTIFACT_TAG:-}}"
  : "${IMAGE_TAG:?请填写 IMAGE_TAG 镜像版本}"
  [[ "$IMAGE_TAG" =~ ^[a-zA-Z0-9_][a-zA-Z0-9_.-]{0,127}$ ]] || {
    echo "镜像版本格式不正确" >&2; exit 1;
  }
  : "${DOCKER_REGISTRY:?缺少 DOCKER_REGISTRY}"
  command -v docker >/dev/null
  command -v aws >/dev/null
fi

if [[ "$MODULE_NAME" == tio-mg-all ]]; then
  export JAVA_HOME="${JDK_HOME:-${JAVA_HOME:-/usr/lib/jvm/java-8-openjdk-amd64}}"
  test -x "$JAVA_HOME/bin/java"
  export PATH="$JAVA_HOME/bin:$PATH"
  "$JAVA_HOME/bin/java" -version 2>&1 | grep -q 'version "1.8.' || {
    echo "后端要求 Java 8，请设置 JDK_HOME" >&2; exit 1;
  }
  mvn -B -f parent/pom.xml clean install -P linux -DskipTests
  mvn -B -f all/pom.xml clean install -P linux -DskipTests
  config_dir=all/target/tio-mg-all/config
  test -f "$config_dir/app.properties"
  test -f "$config_dir/app-prod.properties"
  # 使用临时文件同时兼容 GNU/BSD sed；不修改源代码中的配置。
  sed 's/^tio\.env=dev/tio.env=prod/' "$config_dir/app.properties" > "$config_dir/app.properties.tmp"
  mv "$config_dir/app.properties.tmp" "$config_dir/app.properties"
  sed 's/127\.0\.0\.1/host.docker.internal/g' "$config_dir/app-prod.properties" > "$config_dir/app-prod.properties.tmp"
  mv "$config_dir/app-prod.properties.tmp" "$config_dir/app-prod.properties"
else
  (
    cd pc-html
    # Vue CLI 4 / webpack 4 在 Node 17+ 上需要 OpenSSL 兼容选项。
    node_major="$(node -p 'process.versions.node.split(".")[0]')"
    if (( node_major >= 17 )); then
      export NODE_OPTIONS="${NODE_OPTIONS:-} --openssl-legacy-provider"
    fi
    if [[ -f package-lock.json ]]; then npm ci; else npm install; fi
    npm run build
    test -f dist/index.html
  )
fi

if [[ "$BUILD_PUSH" == true ]]; then
  aws_args=(--region "${AWS_REGION:-ap-northeast-1}")
  if [[ -n "${AWS_PROFILE:-}" ]]; then aws_args+=(--profile "$AWS_PROFILE"); fi
  aws "${aws_args[@]}" ecr get-login-password |
    docker login --username AWS --password-stdin "$DOCKER_REGISTRY"
  docker buildx build --platform linux/amd64 \
    --tag "$DOCKER_REGISTRY/$image_name:$IMAGE_TAG" --push "$module_dir"
  echo "镜像已推送: $DOCKER_REGISTRY/$image_name:$IMAGE_TAG"
else
  echo "编译完成: $MODULE_NAME"
fi
