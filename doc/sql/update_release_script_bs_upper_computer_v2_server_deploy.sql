UPDATE release_scripts
SET description = '上位机 V2 Server/Web 共用服务器部署脚本，按 MODULE_NAME 独立部署当前模块',
    default_variables = jsonb_build_object(
        'REMOTE_HOST', '16.163.154.121',
        'REMOTE_PORT', '22',
        'REMOTE_USER', 'devops',
        'REMOTE_PASSWORD', '',
        'DEPLOY_ROOT', '/opt/bs-upper-computer-v2',
        'SERVER_REMOTE_DIR', '/opt/python/bs_upper_computer_v2/server',
        'WEB_REMOTE_DIR', '/opt/1panel/www/sites/upper.bslbatt.org/index',
        'REMOTE_APP_SCRIPT', '/opt/python/sh/app.sh',
        'SERVER_VENV_DIR', '/opt/python/venv/bs-server',
        'NPM_INSTALL_COMMAND', 'npm ci --include=dev',
        'WEB_BUILD_COMMAND', 'npm run build:pro',
        'KEEP_REMOTE_BACKUP', '1'
    ),
    content = $aether_script$#!/usr/bin/env bash
# 上位机 V2 Server/Web 共用部署脚本。
# 平台分别为 server、web 注入 MODULE_NAME 和 MODULE_WORK_DIR，本次只部署当前模块。
set -Eeuo pipefail

: "${WORK_DIR:?缺少平台变量 WORK_DIR}"
: "${MODULE_NAME:?缺少平台变量 MODULE_NAME}"
: "${MODULE_WORK_DIR:?缺少平台变量 MODULE_WORK_DIR}"
: "${REMOTE_HOST:?缺少 REMOTE_HOST}"
: "${REMOTE_USER:?缺少 REMOTE_USER}"
: "${REMOTE_PASSWORD:?请在运行确认页填写 REMOTE_PASSWORD}"

REMOTE_PORT="${REMOTE_PORT:-22}"
DEPLOY_ROOT="${DEPLOY_ROOT:-/opt/bs-upper-computer-v2}"
REMOTE_APP_SCRIPT="${REMOTE_APP_SCRIPT:-/opt/python/sh/app.sh}"
SERVER_VENV_DIR="${SERVER_VENV_DIR:-/opt/python/venv/bs-server}"
NPM_INSTALL_COMMAND="${NPM_INSTALL_COMMAND:-npm ci --include=dev}"
WEB_BUILD_COMMAND="${WEB_BUILD_COMMAND:-npm run build:pro}"
KEEP_REMOTE_BACKUP="${KEEP_REMOTE_BACKUP:-1}"
REMOTE_SUDO_PASSWORD="${REMOTE_SUDO_PASSWORD:-${REMOTE_PASSWORD}}"
SERVER_REMOTE_DIR="${SERVER_REMOTE_DIR:-${DEPLOY_ROOT}/server}"
WEB_REMOTE_DIR="${WEB_REMOTE_DIR:-${DEPLOY_ROOT}/web}"

log() {
    printf '[%s] %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*"
}

fail() {
    log "错误: $*" >&2
    exit 1
}

require_cmd() {
    command -v "$1" >/dev/null 2>&1 || fail "缺少本地命令: $1"
}

[[ "$REMOTE_USER" =~ ^[a-zA-Z_][a-zA-Z0-9_-]*$ ]] || fail "REMOTE_USER 无效"
[[ "$REMOTE_HOST" =~ ^[a-zA-Z0-9][a-zA-Z0-9.-]*$ ]] || fail "REMOTE_HOST 无效"
[[ "$REMOTE_PORT" =~ ^[0-9]{1,5}$ ]] || fail "REMOTE_PORT 无效"
(( 10#$REMOTE_PORT >= 1 && 10#$REMOTE_PORT <= 65535 )) || fail "REMOTE_PORT 超出有效范围"
[[ "$DEPLOY_ROOT" == /* && "$DEPLOY_ROOT" != "/" ]] || fail "DEPLOY_ROOT 必须是非根目录的绝对路径"
[[ "$SERVER_REMOTE_DIR" == /* && "$SERVER_REMOTE_DIR" != "/" ]] || fail "SERVER_REMOTE_DIR 必须是非根目录的绝对路径"
[[ "$WEB_REMOTE_DIR" == /* && "$WEB_REMOTE_DIR" != "/" ]] || fail "WEB_REMOTE_DIR 必须是非根目录的绝对路径"
[[ "$REMOTE_APP_SCRIPT" == /* ]] || fail "REMOTE_APP_SCRIPT 必须是绝对路径"
[[ "$SERVER_VENV_DIR" == /* && "$SERVER_VENV_DIR" != "/" ]] || fail "SERVER_VENV_DIR 必须是非根目录的绝对路径"
[[ "$KEEP_REMOTE_BACKUP" == "0" || "$KEEP_REMOTE_BACKUP" == "1" ]] || fail "KEEP_REMOTE_BACKUP 只能是 0 或 1"
[[ -d "$MODULE_WORK_DIR" ]] || fail "模块目录不存在: $MODULE_WORK_DIR"

for command_name in tar sshpass ssh scp mktemp; do
    require_cmd "$command_name"
done

case "$MODULE_NAME" in
    server)
        [[ -f "$MODULE_WORK_DIR/main.py" ]] || fail "Server 缺少 main.py: $MODULE_WORK_DIR"
        [[ -f "$MODULE_WORK_DIR/pyproject.toml" ]] || fail "Server 缺少 pyproject.toml: $MODULE_WORK_DIR"
        [[ -d "$MODULE_WORK_DIR/src" ]] || fail "Server 缺少 src: $MODULE_WORK_DIR"
        ;;
    web)
        require_cmd node
        require_cmd npm
        [[ -f "$MODULE_WORK_DIR/package.json" ]] || fail "Web 缺少 package.json: $MODULE_WORK_DIR"
        [[ -f "$MODULE_WORK_DIR/package-lock.json" ]] || fail "Web 缺少 package-lock.json: $MODULE_WORK_DIR"
        ;;
    *)
        fail "不支持的模块: ${MODULE_NAME}，仅支持 server 或 web"
        ;;
esac

local_tmp=$(mktemp -d "${TMPDIR:-/tmp}/bs-upper-v2.XXXXXXXX")
remote_tmp=""
export SSHPASS="$REMOTE_PASSWORD"
ssh_options=(
    -o StrictHostKeyChecking=accept-new
    -o ConnectTimeout=15
    -o ServerAliveInterval=15
    -o ServerAliveCountMax=3
)
remote="$REMOTE_USER@$REMOTE_HOST"

ssh_run() {
    sshpass -e "$@"
}

cleanup() {
    rm -rf -- "$local_tmp"
    if [[ -n "$remote_tmp" && "$remote_tmp" =~ ^/tmp/bs-upper-v2\.[a-zA-Z0-9]+$ ]]; then
        ssh_run ssh -T -p "$REMOTE_PORT" "${ssh_options[@]}" "$remote" \
            "rm -rf -- '$remote_tmp'" >/dev/null 2>&1 || true
    fi
}
trap cleanup EXIT

remote_tmp=$(ssh_run ssh -T -p "$REMOTE_PORT" "${ssh_options[@]}" "$remote" \
    'mktemp -d /tmp/bs-upper-v2.XXXXXXXX')
[[ "$remote_tmp" =~ ^/tmp/bs-upper-v2\.[a-zA-Z0-9]+$ ]] || fail "远端临时目录返回值无效"

run_remote_deploy() {
    local archive_path="$1"
    local archive_name="$2"

    log "上传 ${archive_name}: ${REMOTE_HOST}:${remote_tmp}/${archive_name}"
    ssh_run scp -P "$REMOTE_PORT" "${ssh_options[@]}" \
        "$archive_path" "$remote:$remote_tmp/$archive_name"

    {
        printf 'MODULE_NAME=%q\n' "$MODULE_NAME"
        printf 'DEPLOY_ROOT=%q\n' "$DEPLOY_ROOT"
        printf 'SERVER_REMOTE_DIR=%q\n' "$SERVER_REMOTE_DIR"
        printf 'WEB_REMOTE_DIR=%q\n' "$WEB_REMOTE_DIR"
        printf 'REMOTE_APP_SCRIPT=%q\n' "$REMOTE_APP_SCRIPT"
        printf 'SERVER_VENV_DIR=%q\n' "$SERVER_VENV_DIR"
        printf 'REMOTE_TMP=%q\n' "$remote_tmp"
        printf 'ARCHIVE_NAME=%q\n' "$archive_name"
        printf 'KEEP_REMOTE_BACKUP=%q\n' "$KEEP_REMOTE_BACKUP"
        printf 'REMOTE_SUDO_PASSWORD=%q\n' "$REMOTE_SUDO_PASSWORD"
        cat <<'REMOTE_SCRIPT'
set -Eeuo pipefail

as_root() {
    if [[ "$(id -u)" == "0" ]]; then
        "$@"
    else
        command -v sudo >/dev/null 2>&1 || {
            echo "远端缺少 sudo，且当前用户不是 root" >&2
            return 1
        }
        printf '%s\n' "$REMOTE_SUDO_PASSWORD" | sudo -S -p '' "$@"
    fi
}

timestamp=$(date +%Y%m%d%H%M%S)
archive_path="$REMOTE_TMP/$ARCHIVE_NAME"
command -v tar >/dev/null 2>&1 || { echo "远端缺少 tar" >&2; exit 1; }
tar -tzf "$archive_path" >/dev/null

case "$MODULE_NAME" in
    server)
        [[ -f "$REMOTE_APP_SCRIPT" && -r "$REMOTE_APP_SCRIPT" ]] || {
            echo "外部启动脚本不存在或不可读: $REMOTE_APP_SCRIPT" >&2
            exit 1
        }
        bash -n "$REMOTE_APP_SCRIPT"
        configured_app_dir=$(awk -F= '
            /^[[:space:]]*APP_DIR[[:space:]]*=/ {
                value=$0
                sub(/^[^=]*=[[:space:]]*/, "", value)
                gsub(/^[[:space:]]+|[[:space:]]+$/, "", value)
                quote=substr(value, 1, 1)
                if ((quote == "\"" || quote == sprintf("%c", 39)) &&
                    substr(value, length(value), 1) == quote) {
                    value=substr(value, 2, length(value) - 2)
                }
                print value
                exit
            }
        ' "$REMOTE_APP_SCRIPT")
        [[ "$configured_app_dir" == "$SERVER_REMOTE_DIR" ]] || {
            echo "外部启动脚本 APP_DIR 不正确: ${configured_app_dir:-未配置}" >&2
            echo "期望值: $SERVER_REMOTE_DIR" >&2
            exit 1
        }

        backup_dir="${SERVER_REMOTE_DIR}.bak.${timestamp}"
        if [[ -d "$SERVER_REMOTE_DIR" ]]; then
            if [[ "$KEEP_REMOTE_BACKUP" == "1" ]]; then
                as_root rm -rf -- "$backup_dir"
                as_root cp -a -- "$SERVER_REMOTE_DIR" "$backup_dir"
                echo "已备份 Server: $backup_dir"
            fi
            as_root find "$SERVER_REMOTE_DIR" -mindepth 1 -maxdepth 1 \
                ! -name logs -exec rm -rf -- {} +
        else
            as_root mkdir -p -- "$SERVER_REMOTE_DIR"
        fi
        as_root tar -xzf "$archive_path" -C "$SERVER_REMOTE_DIR"
        as_root chown -R "$(id -un):$(id -gn)" "$SERVER_REMOTE_DIR"
        as_root find "$SERVER_REMOTE_DIR" -type d -exec chmod 755 {} +
        as_root find "$SERVER_REMOTE_DIR" -type f -exec chmod 644 {} +
        [[ -d "$SERVER_VENV_DIR" ]] || {
            echo "Server 虚拟环境不存在: $SERVER_VENV_DIR" >&2
            exit 1
        }
        as_root chown -R "$(id -un):$(id -gn)" "$SERVER_VENV_DIR"
        bash "$REMOTE_APP_SCRIPT" < /dev/null
        echo "Server 部署成功: $SERVER_REMOTE_DIR"
        ;;
    web)
        backup_dir="${WEB_REMOTE_DIR}.bak.${timestamp}"
        if [[ -d "$WEB_REMOTE_DIR" ]]; then
            if [[ "$KEEP_REMOTE_BACKUP" == "1" ]]; then
                as_root rm -rf -- "$backup_dir"
                as_root cp -a -- "$WEB_REMOTE_DIR" "$backup_dir"
                echo "已备份 Web: $backup_dir"
            fi
            as_root find "$WEB_REMOTE_DIR" -mindepth 1 -maxdepth 1 -exec rm -rf -- {} +
        else
            as_root mkdir -p -- "$WEB_REMOTE_DIR"
        fi
        as_root tar -xzf "$archive_path" -C "$WEB_REMOTE_DIR"
        [[ -f "$WEB_REMOTE_DIR/index.html" ]] || {
            echo "Web 部署后缺少 index.html: $WEB_REMOTE_DIR" >&2
            exit 1
        }
        as_root find "$WEB_REMOTE_DIR" -type d -exec chmod 755 {} +
        as_root find "$WEB_REMOTE_DIR" -type f -exec chmod 644 {} +
        echo "Web 部署成功: $WEB_REMOTE_DIR"
        ;;
    *)
        echo "不支持的模块: $MODULE_NAME" >&2
        exit 1
        ;;
esac

rm -rf -- "$REMOTE_TMP"
REMOTE_SCRIPT
    } | ssh_run ssh -T -p "$REMOTE_PORT" "${ssh_options[@]}" "$remote" 'bash -s'
    remote_tmp=""
}

case "$MODULE_NAME" in
    server)
        archive="$local_tmp/server.tar.gz"
        log "打包 Server: $MODULE_WORK_DIR"
        tar -C "$MODULE_WORK_DIR" -czf "$archive" \
            --exclude='.venv' \
            --exclude='venv' \
            --exclude='__pycache__' \
            --exclude='*.pyc' \
            --exclude='*.pyo' \
            --exclude='.pytest_cache' \
            --exclude='.mypy_cache' \
            --exclude='.ruff_cache' \
            --exclude='*.egg-info' \
            --exclude='build' \
            --exclude='dist' \
            --exclude='logs' \
            --exclude='*.log' \
            --exclude='server.pid' \
            --exclude='.deploy.lock' \
            --exclude='.DS_Store' \
            --exclude='devops/app.sh' \
            .
        tar -tzf "$archive" >/dev/null
        run_remote_deploy "$archive" "server.tar.gz"
        ;;
    web)
        log "Web 当前目录: $MODULE_WORK_DIR"
        log "Node: $(node -v)，npm: $(npm -v)"
        (
            cd "$MODULE_WORK_DIR"
            log "安装依赖: $NPM_INSTALL_COMMAND"
            env -u BASH_ENV bash -c "$NPM_INSTALL_COMMAND"
            log "构建 Web: $WEB_BUILD_COMMAND"
            env -u BASH_ENV bash -c "$WEB_BUILD_COMMAND"
        )
        [[ -f "$MODULE_WORK_DIR/dist/index.html" ]] || fail "Web 构建产物缺少 dist/index.html"
        archive="$local_tmp/web.tar.gz"
        log "打包 Web dist 内容"
        tar -C "$MODULE_WORK_DIR/dist" -czf "$archive" .
        tar -tzf "$archive" >/dev/null
        run_remote_deploy "$archive" "web.tar.gz"
        ;;
esac

log "部署完成: module=$MODULE_NAME, host=$REMOTE_HOST, root=$DEPLOY_ROOT"
$aether_script$,
    updated_at = NOW()
WHERE name = '贝斯-上位机V2-服务器部署'
RETURNING id, name, description, default_variables, octet_length(content) AS content_bytes;
