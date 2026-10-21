#!/usr/bin/env bash
set -Eeuo pipefail
stage_name='初始化'
trap 'rc=$?; echo "发布失败：阶段=$stage_name，行=$LINENO，退出码=$rc" >&2; exit "$rc"' ERR
: "${WORK_DIR:?Missing WORK_DIR}"
: "${MODULE_NAME:?Missing MODULE_NAME}"
: "${REMOTE_HOST:?Set actual server IP or hostname}"
REMOTE_PORT=${REMOTE_PORT:-22}
REMOTE_USER=${REMOTE_USER:-root}
case "$MODULE_NAME" in zungui-api|zungui-web|zungui-message) ;; *) echo 'Invalid module' >&2; exit 1;; esac
[[ $REMOTE_HOST =~ ^[a-zA-Z0-9][a-zA-Z0-9.-]*$ && $REMOTE_USER =~ ^[a-zA-Z_][a-zA-Z0-9_-]*$ && $REMOTE_PORT =~ ^[0-9]+$ ]] || { echo 'Invalid SSH parameters' >&2; exit 1; }
case "$MODULE_NAME" in
    zungui-api) default_port=21001;;
    zungui-web) default_port=21002;;
    zungui-message) default_port=21003;;
esac
SERVER_PORT=${SERVER_PORT:-$default_port}
[[ $SERVER_PORT =~ ^[1-9][0-9]{0,4}$ ]] && ((SERVER_PORT <= 65535)) || { echo 'Invalid SERVER_PORT' >&2; exit 1; }
for cmd in mvn java ssh scp unzip shasum; do command -v "$cmd" >/dev/null; done
java -version 2>&1 | grep -q 'version "1\.8\.' || { echo 'Build requires Java 8' >&2; exit 1; }
cd "$WORK_DIR"
# Portable checkout lock (Linux/macOS). Remove a stale lock only after checking no build runs.
build_lock="$WORK_DIR/.zungui-build.lock"
mkdir "$build_lock" 2>/dev/null || { echo 'Another build uses this checkout (or stale .zungui-build.lock)' >&2; exit 1; }
stage=''
cleanup() {
    if [[ -n $stage ]]; then ssh_run "rm -f '$stage/package.war'; rmdir '$stage'" || true; fi
    rmdir "$build_lock"
}
trap cleanup EXIT
stage_name='Maven 构建'
mvn -B -pl "$MODULE_NAME" -am clean package -DskipTests
stage_name='WAR 产物校验'
war="$MODULE_NAME/target/$MODULE_NAME-1.0.war"
[[ -s $war ]] || { echo "Missing $war" >&2; exit 1; }
manifest=$(unzip -p "$war" META-INF/MANIFEST.MF | tr -d '\r')
grep -q '^Main-Class: org.springframework.boot.loader.WarLauncher$' <<< "$manifest"
case "$MODULE_NAME" in
    zungui-api) main=com.zungui.api.ApiMain;;
    zungui-web) main=com.zungui.web.Main;;
    zungui-message) main=com.zungui.message.MessageMain;;
esac
grep -Fxq "Start-Class: $main" <<< "$manifest"
checksum=$(shasum -a 256 "$war" | awk '{print $1}')
ssh_opts=(-o BatchMode=yes -o StrictHostKeyChecking=yes -o ConnectTimeout=15)
auth=()
if [[ -n ${REMOTE_PASSWORD:-} ]]; then
    command -v sshpass >/dev/null
    export SSHPASS=$REMOTE_PASSWORD
    auth=(sshpass -e)
    ssh_opts[1]=BatchMode=no
fi
remote="$REMOTE_USER@$REMOTE_HOST"
ssh_run() { "${auth[@]}" ssh "${ssh_opts[@]}" -p "$REMOTE_PORT" "$remote" "$@"; }
# Preflight before upload; SSH host key must already be trusted by the CI account.
stage_name='目标服务器检查'
echo "检查目标服务器: $remote:$REMOTE_PORT /opt/zungui"
ssh_run 'set -e
[ -x /opt/zungui/app.sh ] || { echo "缺少可执行脚本 /opt/zungui/app.sh，请先安装" >&2; exit 1; }
[ -w /opt/zungui ] || { echo "当前 SSH 用户无法写入 /opt/zungui" >&2; exit 1; }
for cmd in flock ss sha256sum; do
    command -v "$cmd" >/dev/null || { echo "目标服务器缺少命令: $cmd" >&2; exit 1; }
done
java -version 2>&1 | grep '\''version "1\.8\.'\'' || { echo "目标服务器需要 Java 8" >&2; exit 1; }
echo "目标服务器检查通过"'
stage_name='上传 WAR'
stage=$(ssh_run 'mktemp -d /opt/zungui/.upload.XXXXXXXX')
[[ $stage =~ ^/opt/zungui/\.upload\.[a-zA-Z0-9]+$ ]] || { echo 'Invalid staging directory' >&2; exit 1; }
"${auth[@]}" scp "${ssh_opts[@]}" -P "$REMOTE_PORT" "$war" "$remote:$stage/package.war"
stage_name='远端替换和启动'
ssh_run "bash -s -- '$MODULE_NAME' '$stage' '$checksum' '$SERVER_PORT'" <<'REMOTE'
set -Eeuo pipefail
module=$1
stage=$2
checksum=$3
port=$4
cd /opt/zungui
mkdir -p run backups
exec 9>"run/$module.deploy.lock"
flock -n 9 || { echo 'Module deployment already running' >&2; exit 1; }
printf '%s  %s\n' "$checksum" "$stage/package.war" | sha256sum -c -
if [[ -f $module.war ]]; then
    cp -p "$module.war" "backups/$module.$(date +%Y%m%d%H%M%S).$$.war"
fi
# Stop before replacing the archive: an existing JVM may lazily read its WAR.
./app.sh stop "$module" 9>&-
mv "$stage/package.war" "$module.war"
chmod 644 "$module.war"
./app.sh start "$module" "$port" 9>&-
echo "Deployed $module"
REMOTE
