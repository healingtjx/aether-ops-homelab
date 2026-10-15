UPDATE release_scripts
SET content = $script$
#!/usr/bin/env bash
set -Eeuo pipefail

: "${BUILD_DIR:?缺少 BUILD_DIR}"
: "${PROJECT_NAME:?缺少 PROJECT_NAME，对应远程 projects.conf 第一列}"
: "${REMOTE_JAR_NAME:?缺少 REMOTE_JAR_NAME，对应远程 projects.conf 第二列文件名}"
: "${REMOTE_HOST:?缺少 REMOTE_HOST}"
: "${REMOTE_USER:?缺少 REMOTE_USER}"
: "${REMOTE_PASSWORD:?缺少 REMOTE_PASSWORD}"

REMOTE_PORT="${REMOTE_PORT:-22}"
REMOTE_DIR="${REMOTE_DIR:-/opt/bsjava}"
REMOTE_SUDO_PASSWORD="${REMOTE_SUDO_PASSWORD:-$REMOTE_PASSWORD}"
REMOTE_APP_ACTION="${REMOTE_APP_ACTION:-restart}"

if [ -n "${WORK_DIR:-}" ] && [ "${BUILD_DIR#/}" = "$BUILD_DIR" ]; then
    cd "${WORK_DIR}/${BUILD_DIR}"
else
    cd "$BUILD_DIR"
fi

if [ -x "./mvnw" ]; then
    ./mvnw -DskipTests clean package
elif [ -f "pom.xml" ]; then
    mvn -DskipTests clean package
else
    echo "未找到 Maven 项目: $PWD"
    exit 1
fi

JAR_FILE=$(find target -maxdepth 1 -type f -name "*.jar" \
    ! -name "*-sources.jar" \
    ! -name "*-javadoc.jar" \
    ! -name "original-*.jar" \
    | head -n 1)

if [ -z "$JAR_FILE" ]; then
    echo "未找到 JAR: $PWD/target"
    exit 1
fi

REMOTE_TMP="/tmp/aether_ops_${PROJECT_NAME}_${REMOTE_JAR_NAME}.tmp"
REMOTE_TARGET="${REMOTE_DIR}/${REMOTE_JAR_NAME}"

command -v sshpass >/dev/null 2>&1 || {
    echo "缺少命令: sshpass"
    exit 1
}

quote() {
    printf "%q" "$1"
}

echo "准备上传: $JAR_FILE -> ${REMOTE_HOST}:${REMOTE_TMP}"

sshpass -p "$REMOTE_PASSWORD" scp \
    -P "$REMOTE_PORT" \
    -o StrictHostKeyChecking=accept-new \
    -o ConnectTimeout=15 \
    "$JAR_FILE" \
    "${REMOTE_USER}@${REMOTE_HOST}:${REMOTE_TMP}"

echo "上传完成，准备部署到: ${REMOTE_TARGET}"

REMOTE_CMD="
set -Eeuo pipefail

sudo_run() {
    printf '%s\n' $(quote "$REMOTE_SUDO_PASSWORD") | sudo -S -p '' \"\$@\"
}

echo '远程已连接: '\"\$(hostname)\"
echo '检查上传文件: '$(quote "$REMOTE_TMP")

if [ ! -f $(quote "$REMOTE_TMP") ]; then
    echo '上传文件不存在: '$(quote "$REMOTE_TMP")
    exit 1
fi

sudo_run mkdir -p $(quote "$REMOTE_DIR")
sudo_run mv $(quote "$REMOTE_TMP") $(quote "$REMOTE_TARGET")
sudo_run chmod 644 $(quote "$REMOTE_TARGET")

cd $(quote "$REMOTE_DIR")

echo '当前目录: '\"\$(pwd)\"
echo '目标文件:'
ls -lh $(quote "$REMOTE_JAR_NAME")

echo '执行: sudo ./app.sh $(quote "$REMOTE_APP_ACTION") $(quote "$PROJECT_NAME")'
sudo_run ./app.sh $(quote "$REMOTE_APP_ACTION") $(quote "$PROJECT_NAME")

echo '发布完成: '$(quote "$PROJECT_NAME")
"

sshpass -p "$REMOTE_PASSWORD" ssh -T \
    -p "$REMOTE_PORT" \
    -o StrictHostKeyChecking=accept-new \
    -o ConnectTimeout=15 \
    "${REMOTE_USER}@${REMOTE_HOST}" \
    "$REMOTE_CMD"
$script$,
    updated_at = NOW()
WHERE name = '贝斯-bms-服务端';
