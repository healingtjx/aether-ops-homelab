#!/usr/bin/env bash
set -Eeuo pipefail

# 通用 JAR 发布脚本
# 运行位置：Aether Ops CI 拉取后的应用源码目录。
# 默认项目名取当前目录名；如需覆盖，可在脚本顶部或 CI 环境中设置 PROJECT_NAME。
# 默认执行 sudo ./app.sh ${PROJECT_NAME}；如果远端 app.sh 需要 restart/start，可设置 REMOTE_APP_ACTION=restart。

REMOTE_HOST="${REMOTE_HOST:-172.16.10.200}"
REMOTE_PORT="${REMOTE_PORT:-22}"
REMOTE_USER="${REMOTE_USER:-tjc}"
REMOTE_PASSWORD="${REMOTE_PASSWORD:-tangjiaxing}"
REMOTE_DIR="${REMOTE_DIR:-/opt/bsjava}"
PROJECT_NAME="${PROJECT_NAME:-$(basename "$PWD")}"
REMOTE_JAR_NAME="${REMOTE_JAR_NAME:-}"
REMOTE_APP_ACTION="${REMOTE_APP_ACTION:-}"

log() {
    printf '[%s] %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*"
}

require_cmd() {
    if ! command -v "$1" >/dev/null 2>&1; then
        log "缺少命令: $1"
        exit 1
    fi
}

build_project() {
    if [ -x "./mvnw" ]; then
        log "使用 Maven Wrapper 打包"
        ./mvnw -DskipTests clean package
        return
    fi

    if [ -f "pom.xml" ]; then
        require_cmd mvn
        log "使用 Maven 打包"
        mvn -DskipTests clean package
        return
    fi

    if [ -x "./gradlew" ]; then
        log "使用 Gradle Wrapper 打包"
        ./gradlew clean build -x test
        return
    fi

    if [ -f "build.gradle" ] || [ -f "build.gradle.kts" ]; then
        require_cmd gradle
        log "使用 Gradle 打包"
        gradle clean build -x test
        return
    fi

    log "未找到 pom.xml、mvnw、gradlew 或 build.gradle，无法打包"
    exit 1
}

find_jar() {
    local jar_file

    jar_file=$(
        {
            if [ -d "target" ]; then
                find target -maxdepth 2 -type f -name "*.jar" \
                    ! -name "*-sources.jar" \
                    ! -name "*-javadoc.jar" \
                    ! -name "original-*.jar"
            fi
            if [ -d "build/libs" ]; then
                find build/libs -maxdepth 2 -type f -name "*.jar" \
                    ! -name "*-sources.jar" \
                    ! -name "*-javadoc.jar" \
                    ! -name "original-*.jar"
            fi
        } | while IFS= read -r file; do
            printf '%s\t%s\n' "$(stat -f '%m' "$file" 2>/dev/null || stat -c '%Y' "$file")" "$file"
        done | sort -rn | head -n 1 | cut -f2-)

    if [ -z "$jar_file" ]; then
        log "未找到可发布的 JAR 产物"
        exit 1
    fi

    printf '%s\n' "$jar_file"
}

upload_and_restart() {
    local jar_file=$1
    local jar_name=$2
    local remote_target="${REMOTE_DIR}/${jar_name}"
    local remote_tmp="/tmp/aether_ops_${PROJECT_NAME}_${jar_name}.tmp"
    local remote_app_args="'${PROJECT_NAME}'"

    if [ -n "$REMOTE_APP_ACTION" ]; then
        remote_app_args="'${REMOTE_APP_ACTION}' '${PROJECT_NAME}'"
    fi

    require_cmd sshpass
    require_cmd scp
    require_cmd ssh

    log "创建远程目录: ${REMOTE_USER}@${REMOTE_HOST}:${REMOTE_DIR}"
    sshpass -p "$REMOTE_PASSWORD" ssh \
        -p "$REMOTE_PORT" \
        -o StrictHostKeyChecking=accept-new \
        "${REMOTE_USER}@${REMOTE_HOST}" \
        "printf '%s\n' '$REMOTE_PASSWORD' | sudo -S mkdir -p '$REMOTE_DIR'"

    log "上传 JAR: ${jar_file} -> ${REMOTE_HOST}:${remote_tmp}"
    sshpass -p "$REMOTE_PASSWORD" scp \
        -P "$REMOTE_PORT" \
        -o StrictHostKeyChecking=accept-new \
        "$jar_file" \
        "${REMOTE_USER}@${REMOTE_HOST}:${remote_tmp}"

    log "替换远程 JAR 并执行 app.sh: ${PROJECT_NAME}"
    sshpass -p "$REMOTE_PASSWORD" ssh \
        -p "$REMOTE_PORT" \
        -o StrictHostKeyChecking=accept-new \
        "${REMOTE_USER}@${REMOTE_HOST}" \
        "set -e; cd '$REMOTE_DIR'; printf '%s\n' '$REMOTE_PASSWORD' | sudo -S mv '$remote_tmp' '$remote_target'; printf '%s\n' '$REMOTE_PASSWORD' | sudo -S ./app.sh ${remote_app_args}"
}

main() {
    log "开始发布项目: ${PROJECT_NAME}"
    build_project

    local jar_file
    jar_file=$(find_jar)

    local jar_name
    jar_name="${REMOTE_JAR_NAME:-$(basename "$jar_file")}"

    upload_and_restart "$jar_file" "$jar_name"
    log "发布完成: ${PROJECT_NAME}"
}

main "$@"
