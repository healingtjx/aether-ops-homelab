#!/usr/bin/env bash
# 测试环境静态站点发布：目标目录必须专用于本站点，内容会被清空，无备份。
set -Eeuo pipefail
stage_name='初始化'
trap 'rc=$?; echo "发布失败：阶段=${stage_name}，行=${LINENO}，退出码=${rc}" >&2; exit "$rc"' ERR
: "${WORK_DIR:?Missing WORK_DIR}"
: "${MODULE_NAME:?Missing MODULE_NAME}"
: "${REMOTE_HOST:?Missing REMOTE_HOST}"
REMOTE_PORT=${REMOTE_PORT:-22}
REMOTE_USER=${REMOTE_USER:-tjx}
AJAX_DOMAIN=${AJAX_DOMAIN:-http://192.168.5.4:21001}
case "$MODULE_NAME" in
    zungui-pc-admin) source_path=zungui-pc/back-admin; entry=Login.html; default_port=21004; dirs=(DNA INTEGRATE PC Views Public);;
    zungui-pc-auction) source_path=zungui-pc/back-auction; entry=login.html; default_port=21005; dirs=(Public PuData pdfh5 views);;
    *) echo '不支持的 MODULE_NAME' >&2; exit 1;;
esac
SERVER_PORT=${SERVER_PORT:-$default_port}
[[ $SERVER_PORT =~ ^[1-9][0-9]{0,4}$ ]] && ((SERVER_PORT <= 65535)) || { echo 'SERVER_PORT 不合法' >&2; exit 1; }
DEPLOY_DIR=${DEPLOY_DIR-/opt/zungui/$MODULE_NAME}
# 仅允许 /opt/zungui 下的独立站点目录，禁止覆盖后端或运维目录。
[[ $DEPLOY_DIR =~ ^/opt/zungui/[a-zA-Z0-9][a-zA-Z0-9_-]*$ ]] || { echo 'DEPLOY_DIR 必须是 /opt/zungui/<独立站点目录>' >&2; exit 1; }
case "${DEPLOY_DIR##*/}" in backups|run|logs|tmp|certs|fonts|zungui-api|zungui-web|zungui-message) echo '禁止使用此部署目录' >&2; exit 1;; esac
[[ $REMOTE_HOST =~ ^[a-zA-Z0-9][a-zA-Z0-9.-]*$ && $REMOTE_USER =~ ^[a-zA-Z_][a-zA-Z0-9_-]*$ && $REMOTE_PORT =~ ^[1-9][0-9]{0,4}$ ]] && ((REMOTE_PORT <= 65535)) || { echo 'SSH 参数不合法' >&2; exit 1; }
for cmd in tar ssh scp shasum cp find sed grep; do command -v "$cmd" >/dev/null; done
ssh_opts=(-o BatchMode=yes -o StrictHostKeyChecking=yes -o ConnectTimeout=15)
auth=(env)
if [[ -n ${REMOTE_PASSWORD:-} ]]; then
    command -v sshpass >/dev/null
    export SSHPASS=$REMOTE_PASSWORD
    auth=(sshpass -e)
    ssh_opts[1]=BatchMode=no
fi
remote="$REMOTE_USER@$REMOTE_HOST"
ssh_run() { "${auth[@]}" ssh "${ssh_opts[@]}" -p "$REMOTE_PORT" "$remote" "$@"; }
local_stage=$(mktemp -d)
remote_stage=''
cleanup() {
    local rc=$?
    trap - EXIT
    if [[ -n $remote_stage ]]; then ssh_run "rm -rf -- '$remote_stage'" || true; fi
    rm -rf -- "$local_stage"
    exit "$rc"
}
trap cleanup EXIT
stage_name='整理静态资源和替换 API 地址'
# 限制为 HTTP(S) 主机、可选端口和路径，避免把控制字符写入 JavaScript。
[[ $AJAX_DOMAIN =~ ^https?://[a-zA-Z0-9][a-zA-Z0-9.-]*(:[0-9]{1,5})?(/[a-zA-Z0-9._~%/-]*)?$ ]] || { echo 'AJAX_DOMAIN 不是有效的 HTTP(S) API 地址' >&2; exit 1; }
AJAX_DOMAIN=${AJAX_DOMAIN%/}
source_dir="$WORK_DIR/$source_path"
mkdir "$local_stage/site"
for dir in "${dirs[@]}"; do
    [[ -d $source_dir/$dir && ! -L $source_dir/$dir ]] || { echo "缺少静态资源目录：$dir" >&2; exit 1; }
    [[ -z $(find "$source_dir/$dir" -type l -print -quit) ]] || { echo '静态资源不允许包含符号链接' >&2; exit 1; }
    cp -R "$source_dir/$dir" "$local_stage/site/$dir"
done
shopt -s nullglob
html_files=("$source_dir"/*.html)
(( ${#html_files[@]} > 0 )) || { echo '缺少根目录 HTML 文件' >&2; exit 1; }
for html in "${html_files[@]}"; do
    [[ ! -L $html ]] || { echo 'HTML 不允许为符号链接' >&2; exit 1; }
    cp "$html" "$local_stage/site/"
done
find "$local_stage/site" \( -name .svn -o -name .git -o -name .vs -o -name .DS_Store -o -name node_modules \) -prune -exec rm -rf -- {} +
ajax_file="$local_stage/site/Public/JS/ajax.js"
ajax_pattern='var[[:space:]]+ajax_domain[[:space:]]*=[[:space:]]*"[^";]*"[[:space:]]*;'
matches=$(grep -Eo "$ajax_pattern" "$ajax_file" | wc -l | tr -d '[:space:]')
[[ $matches == 1 ]] || { echo 'ajax_domain 必须且只能声明一次' >&2; exit 1; }
# 不使用 sed -i，兼容 macOS 与 Linux，并保留原文件 BOM。
sed -E "s|$ajax_pattern|var ajax_domain = \"$AJAX_DOMAIN\";|" "$ajax_file" > "$local_stage/ajax.js"
mv "$local_stage/ajax.js" "$ajax_file"
[[ -s $local_stage/site/$entry && -s $local_stage/site/index.html ]]
COPYFILE_DISABLE=1 tar -czf "$local_stage/site.tar.gz" -C "$local_stage/site" .
checksum=$(shasum -a 256 "$local_stage/site.tar.gz" | awk '{print $1}')
stage_name='上传静态资源'
remote_stage=$(ssh_run 'mktemp -d /tmp/zungui-pc-upload.XXXXXXXX')
[[ $remote_stage =~ ^/tmp/zungui-pc-upload\.[a-zA-Z0-9]+$ ]] || { remote_stage=''; echo '远端临时目录不合法' >&2; exit 1; }
"${auth[@]}" scp "${ssh_opts[@]}" -P "$REMOTE_PORT" "$local_stage/site.tar.gz" "$remote:$remote_stage/site.tar.gz"
stage_name='校验并覆盖站点文件'
ssh_run "bash -s -- '$remote_stage' '$checksum' '$DEPLOY_DIR' '$entry' '$SERVER_PORT'" <<'REMOTE'
set -Eeuo pipefail
stage=$1
checksum=$2
dest=$3
entry=$4
port=$5
for cmd in sha256sum tar realpath find python3 ps awk readlink nohup curl; do command -v "$cmd" >/dev/null; done
printf '%s  %s\n' "$checksum" "$stage/site.tar.gz" | sha256sum -c -
mkdir "$stage/site"
tar -xzf "$stage/site.tar.gz" --no-same-owner -C "$stage/site"
[[ -s $stage/site/$entry && -s $stage/site/index.html && -s $stage/site/Public/JS/ajax.js ]]
[[ $(realpath -m -- "$dest") == "$dest" && ! -L $dest ]] || { echo '目标目录或父目录不允许为符号链接' >&2; exit 1; }
mkdir -p -- "$dest"
[[ -w $dest ]] || { echo '目标目录不可写' >&2; exit 1; }
# ps -ef 的 CMD 从第 8 列开始；只匹配指定端口的 Python http.server。
# 同时检查 --directory，兼容在站点目录内手动启动且不带 --directory 的进程。
while read -r pid process_dir; do
    [[ -n $pid ]] || continue
    if [[ $process_dir == '-' ]]; then
        process_dir=$(readlink "/proc/$pid/cwd") || { echo "无法确认 PID $pid 的站点目录，请先手动停止该进程" >&2; exit 1; }
    fi
    [[ $process_dir == "$dest" ]] || continue
    echo "停止站点进程：PID=$pid port=$port"
    kill -9 "$pid" || { echo "无法停止 PID $pid，请使用相同用户部署或先手动停止该进程" >&2; exit 1; }
done < <(ps -ef | awk -v port="$port" '
    $8 ~ /(^|\/)python3([.][0-9]+)?$/ && $9 == "-m" && $10 == "http.server" && $11 == port {
        dir = "-"
        for (i = 12; i <= NF; i++) {
            if ($i == "--directory") dir = $(i + 1)
        }
        print $2, dir
    }
')
echo "覆盖站点目录：${dest}（不备份）"
find "$dest" -mindepth 1 -maxdepth 1 -exec rm -rf -- {} +
cp -R "$stage/site/." "$dest/"
find "$dest" -type d -exec chmod 755 {} +
find "$dest" -type f -exec chmod 644 {} +
# 日志位于站点目录之外，避免通过 HTTP 暴露。
log_dir=/opt/zungui/logs
mkdir -p "$log_dir"
log_file="$log_dir/${dest##*/}.http.log"
nohup python3 -m http.server "$port" --bind 0.0.0.0 --directory "$dest" > "$log_file" 2>&1 < /dev/null &
server_pid=$!
ready=false
for attempt in 1 2 3 4 5; do
    sleep 1
    if ! kill -0 "$server_pid" 2>/dev/null; then
        echo "HTTP 服务启动失败，请检查 $log_file（端口可能被占用）" >&2
        exit 1
    fi
    if curl --noproxy '*' -fsS --max-time 2 "http://127.0.0.1:$port/$entry" > /dev/null; then
        ready=true
        break
    fi
done
if [[ $ready != true ]]; then
    kill -9 "$server_pid" 2>/dev/null || true
    echo "HTTP 服务检查失败，请检查 $log_file" >&2
    exit 1
fi
echo "发布成功：目录=$dest 端口=$port PID=$server_pid 日志=$log_file"
REMOTE
