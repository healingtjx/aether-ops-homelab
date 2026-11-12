#!/usr/bin/env bash
# Linux 直接部署：Java + Python 静态服务，不使用镜像。
set -Eeuo pipefail
: "${MODULE_NAME:?缺少 MODULE_NAME}"
: "${REMOTE_HOST:?缺少 REMOTE_HOST}"
REMOTE_USER="${REMOTE_USER:-tjx}"
REMOTE_PORT="${REMOTE_PORT:-22}"
case "$MODULE_NAME" in
  tio-site-all) kind=java; default_port=21008 ;;
  pc-html) kind=html; default_port=21009 ;;
  *) echo '不支持的模块' >&2; exit 1 ;;
esac
DEPLOY_DIR="${DEPLOY_DIR:-/opt/tanliao/server/$kind}"
SERVER_PORT="${SERVER_PORT:-$default_port}"
WS_PORT="${WS_PORT:-21010}"
APP_PORT="${APP_PORT:-21011}"
VIEW_PORT="${VIEW_PORT:-21012}"
BACKEND_PORT="${BACKEND_PORT:-21008}"
FRONTEND_PORT="${FRONTEND_PORT:-21009}"
PUBLIC_HOST="${PUBLIC_HOST:-$REMOTE_HOST}"
[[ "$PUBLIC_HOST" =~ ^[a-zA-Z0-9][a-zA-Z0-9.-]*$ ]]
[[ "$DEPLOY_DIR" == "/opt/tanliao/server/$kind" ]] || { echo '部署目录不正确' >&2; exit 1; }
[[ "$REMOTE_HOST" =~ ^[a-zA-Z0-9][a-zA-Z0-9.-]*$ ]]
[[ "$REMOTE_USER" =~ ^[a-zA-Z_][a-zA-Z0-9_-]*$ ]]
for port in "$SERVER_PORT" "$REMOTE_PORT" "$WS_PORT" "$APP_PORT" "$VIEW_PORT" "$BACKEND_PORT" "$FRONTEND_PORT"; do
  [[ "$port" =~ ^[1-9][0-9]{0,4}$ ]] && (( port <= 65535 ))
done
ssh_opts=(-o BatchMode=yes -o StrictHostKeyChecking=yes -o ConnectTimeout=15)
auth=(env)
if [[ -n "${REMOTE_PASSWORD:-}" ]]; then
  command -v sshpass >/dev/null
  export SSHPASS="$REMOTE_PASSWORD"
  auth=(sshpass -e)
  ssh_opts[1]=BatchMode=no
fi
ssh_run() { "${auth[@]}" ssh "${ssh_opts[@]}" -p "$REMOTE_PORT" "$REMOTE_USER@$REMOTE_HOST" "$@"; }
for cmd in ssh scp tar shasum; do command -v "$cmd" >/dev/null; done
ssh_run "bash -s -- '$kind' '$SERVER_PORT' '$WS_PORT' '$APP_PORT' '$VIEW_PORT'" <<'CHECK'
set -Eeuo pipefail
kind=$1; shift
for port in "$@"; do
  if ss -ltnH "sport = :$port" | grep -q .; then
    echo "端口 $port 已被使用，最终部署前将核对当前模块 PID"
  fi
done
for cmd in flock ss sha256sum tar; do command -v "$cmd" >/dev/null; done
if [[ "$kind" == java ]]; then
  java -version 2>&1 | grep 'version "1\.8\.' >/dev/null || { echo '目标服务器需要 Java 8' >&2; exit 1; }
else
  python3 --version
fi
CHECK
# 后端 Maven reactor 共享目录，避免同时构建污染产物。
mkdir .tanliao-server-build.lock 2>/dev/null || { echo '已有构建任务，或遗留 .tanliao-server-build.lock，请检查' >&2; exit 1; }
local_stage=$(mktemp -d)
remote_stage=''
cleanup() {
  rc=$?
  trap - EXIT
  if [[ -n "$remote_stage" ]]; then ssh_run "rm -f '$remote_stage/package.tar.gz'; rmdir '$remote_stage'" || true; fi
  rm -rf "$local_stage"
  rmdir .tanliao-server-build.lock
  exit "$rc"
}
trap cleanup EXIT
mkdir "$local_stage/package"
if [[ "$kind" == java ]]; then
  export JAVA_HOME="${JDK_HOME:-${JAVA_HOME:-/usr/lib/jvm/java-8-openjdk-amd64}}"
  export PATH="$JAVA_HOME/bin:$PATH"
  java -version 2>&1 | grep 'version "1\.8\.' >/dev/null
  # 与 tanliao-admin 使用同一套 Java 8 父 POM 和依赖，仅修改平台工作副本。
  # RELEASE 父 POM 的插件显式使用 jdk.version=17，maven.compiler.target 无法覆盖。
  python3 - <<'JDK8_PARENT'
from pathlib import Path
import xml.etree.ElementTree as ET

path = Path('parent/pom.xml')
content = path.read_text()
ns = {'m': 'http://maven.apache.org/POM/4.0.0'}
parent = ET.fromstring(content).find('m:parent', ns)
assert parent is not None, '缺少项目父 POM'
assert parent.findtext('m:artifactId', namespaces=ns) == 'tio-zoo-parent', '项目父 POM 已变更，请检查'
version = parent.findtext('m:version', namespaces=ns)
compatible = '3.8.6.v20240801-tanliao-jdk8-1'
assert version in ('3.8.6.v20240801-RELEASE', compatible), '未知父 POM 版本，请检查 Java 8 兼容性'
if version != compatible:
    old = '<version>' + version + '</version>'
    assert content.count(old) == 1, '父 POM 版本定位不唯一'
    path.write_text(content.replace(old, '<version>' + compatible + '</version>'))
print('使用 Java 8 兼容父 POM: ' + compatible)
JDK8_PARENT
  # linux profile 只定义在 all 模块，parent reactor 不传此参数。
  mvn -B -f parent/pom.xml clean install -Djdk.version=1.8 -DskipTests
  mvn -B -f all/pom.xml clean install -P linux -Djdk.version=1.8 -DskipTests
  test -d all/target/tio-site-all/lib
  test -f all/target/tio-site-all/config/app.properties
  cp -R all/target/tio-site-all/lib all/target/tio-site-all/config "$local_stage/package/"
  # 仅修改待上传产物；保留 dev 数据库/Redis 配置，覆盖本应用端口及客户端公布地址。
  python3 - "$local_stage/package/config" "$SERVER_PORT" "$WS_PORT" "$APP_PORT" "$VIEW_PORT" "$PUBLIC_HOST" "$FRONTEND_PORT" <<'PROPERTIES'
import re
import sys
from pathlib import Path

folder = Path(sys.argv[1])
api, ws, app, view, host, frontend = sys.argv[2:]
updates = {
    'app.properties': {'tio.env': 'dev', 'http.api.port': api,
                       'im.port.ws': ws, 'im.port.app': app, 'http.view.port': view,
                       'start.flash.policy.server': '2'},
    # dev 配置在 app.properties 之后加载，必须同步端口以免被源码配置覆盖。
    'app-dev.properties': {'http.api.port': api, 'im.port.ws': ws,
                           'im.port.app': app, 'http.view.port': view,
                           'start.flash.policy.server': '2',
                           'site': 'http://' + host + ':' + frontend,
                           'http.cookie.domain': '',
                           'res.root': '/opt/tanliao/server/upload',
                           'my.ip': host, 'tio.pc.select.sever.force': host,
                           'tio.pc.select.sever.force.port': ws,
                           'tio.clu.select.sever.force': host,
                           'tio.clu.select.sever.force.port': app,
                           'back.server': 'http://127.0.0.1:' + api + '/mytio',
                           'mysql.backup.dir': '/opt/tanliao/server/data/mysqlbackup'},
    'logback.properties': {'log.dir': '/opt/tanliao/server/logs/backend'},
}
for name, values in updates.items():
    path = folder / name
    content = path.read_text()
    for key, value in values.items():
        pattern = r'^' + re.escape(key) + r'\s*=.*$'
        replacement = key + '=' + value
        if re.search(pattern, content, re.M):
            content = re.sub(pattern, lambda match: replacement, content, flags=re.M)
        else:
            content = content.rstrip() + '\n' + replacement + '\n'
    path.write_text(content)
PROPERTIES
else
  API_BASE_URL="${API_BASE_URL:-/mytio}"
  [[ "$API_BASE_URL" == /mytio ]]
  (
    cd pc-html
    # 兼容旧 master 中 service 5.x 与锁文件 4.x 混用的声明；源码修复后不再改写。
    python3 - <<'NPM_MANIFEST'
import json
from pathlib import Path

path = Path('package.json')
content = path.read_text()
package = json.loads(content)
lock_path = Path('package-lock.json')
if lock_path.is_file():
    locked = json.loads(lock_path.read_text()).get('packages', {}).get('', {}).get('devDependencies', {})
    deps = package.get('devDependencies', {})
    if deps.get('@vue/cli-service') == '5.0.6' and locked.get('@vue/cli-service') == '^4.5.0':
        assert deps.get('webpack-dev-server') == '3.11.3', '前端依赖声明已变化，请核对锁文件'
        content = content.replace('"@vue/cli-service": "5.0.6"', '"@vue/cli-service": "^4.5.0"')
        content = content.replace('    "webpack-dev-server": "3.11.3",\n', '')
        assert json.loads(content)['devDependencies'] == locked, '前端依赖与锁文件不一致'
        path.write_text(content)
        print('已按锁文件统一为 Vue CLI 4.5.19 / Webpack 4')
NPM_MANIFEST
    node_major=$(node -p 'process.versions.node.split(".")[0]')
    if (( node_major >= 17 )); then export NODE_OPTIONS="${NODE_OPTIONS:-} --openssl-legacy-provider"; fi
    # Ubuntu 系统 npm 8.5.1 的 make-fetch-happen 缓存代码会抛出 insert 异常。
    # 仅本次构建使用官方 npm，不修改服务器全局工具；保留平台配置的缓存和 npmrc。
    if [[ "$(npm --version)" == 8.5.1 ]]; then
      command -v curl >/dev/null
      mkdir "$local_stage/npm-tool"
      curl -fsSL --connect-timeout 10 --max-time 60 \
        https://registry.npmjs.org/npm/-/npm-8.19.4.tgz -o "$local_stage/npm-tool/npm.tgz"
      tar -xzf "$local_stage/npm-tool/npm.tgz" -C "$local_stage/npm-tool"
      npm_cli="$local_stage/npm-tool/package/bin/npm-cli.js"
      npm_options=()
      if [[ -n "${AETHER_NPM_CACHE_DIRECTORY:-}" ]]; then npm_options+=(--cache "$AETHER_NPM_CACHE_DIRECTORY"); fi
      if [[ -n "${AETHER_NPM_USER_CONFIG_FILE:-}" ]]; then npm_options+=(--userconfig "$AETHER_NPM_USER_CONFIG_FILE"); fi
      npm() { command node "$npm_cli" "${npm_options[@]}" "$@"; }
      echo "本次构建使用官方 npm $(npm --version)"
    fi
    if [[ -f package-lock.json ]]; then npm ci; else npm install; fi
    VUE_APP_apiCtx="$API_BASE_URL" VUE_APP_wsCtx="ws://$PUBLIC_HOST:$WS_PORT" npm run build
  )
  test -s pc-html/dist/index.html
  cp -R pc-html/dist/. "$local_stage/package/"
fi
COPYFILE_DISABLE=1 tar -czf "$local_stage/package.tar.gz" -C "$local_stage/package" .
checksum=$(shasum -a 256 "$local_stage/package.tar.gz" | awk '{print $1}')
remote_stage=$(ssh_run 'mktemp -d /tmp/tanliao-server.XXXXXXXX')
[[ "$remote_stage" =~ ^/tmp/tanliao-server\.[a-zA-Z0-9]+$ ]] || { remote_stage=''; echo '远端临时目录不正确' >&2; exit 1; }
"${auth[@]}" scp "${ssh_opts[@]}" -P "$REMOTE_PORT" "$local_stage/package.tar.gz" "$REMOTE_USER@$REMOTE_HOST:$remote_stage/package.tar.gz"
ssh_run "bash -s -- '$kind' '$DEPLOY_DIR' '$SERVER_PORT' '$remote_stage' '$checksum' '$WS_PORT' '$APP_PORT' '$VIEW_PORT' '$BACKEND_PORT'" <<'REMOTE'
set -Eeuo pipefail
kind=$1
dest=$2
port=$3
stage=$4
checksum=$5
ws_port=$6
app_port=$7
view_port=$8
backend_port=$9
ports=("$port")
if [[ "$kind" == java ]]; then ports+=("$ws_port" "$app_port" "$view_port"); fi
[[ "$kind" == java || "$kind" == html ]]
[[ "$dest" == "/opt/tanliao/server/$kind" ]]
[[ ! -L "$dest" ]]
mkdir -p "$dest" /opt/tanliao/server/run /opt/tanliao/server/logs
if [[ "$kind" == java ]]; then
  mkdir -p /opt/tanliao/server/logs/backend /opt/tanliao/server/data/mysqlbackup /opt/tanliao/server/upload
fi
exec 9>"/opt/tanliao/server/run/$kind.lock"
flock -n 9 || { echo '已有部署任务' >&2; exit 1; }
printf '%s  %s\n' "$checksum" "$stage/package.tar.gz" | sha256sum -c -
pid_file="/opt/tanliao/server/run/$kind.pid"
if [[ -f "$pid_file" ]]; then
  pid=$(cat "$pid_file")
  [[ "$pid" =~ ^[1-9][0-9]*$ ]]
  if kill -0 "$pid" 2>/dev/null; then
    cmd=$(tr '\0' ' ' < "/proc/$pid/cmdline")
    [[ "$cmd" == *"$dest"* ]] || { echo 'PID 不属于当前应用，停止发布' >&2; exit 1; }
    kill "$pid"
    for i in {1..30}; do kill -0 "$pid" 2>/dev/null || break; sleep 1; done
    if kill -0 "$pid" 2>/dev/null; then echo '旧进程未退出，停止发布' >&2; exit 1; fi
  fi
fi
for check_port in "${ports[@]}"; do
  if ss -ltnH "sport = :$check_port" | grep -q .; then echo "端口 $check_port 被其他进程占用" >&2; exit 1; fi
done
# 仅清理当前模块的发布文件，日志与 PID 在独立目录。
if [[ "$kind" == java ]]; then
  rm -rf "$dest/lib" "$dest/config"
else
  find "$dest" -mindepth 1 -maxdepth 1 -exec rm -rf -- {} +
fi
tar -xzf "$stage/package.tar.gz" -C "$dest"
cd "$dest"
if [[ "$kind" == java ]]; then
  nohup java -Xverify:none -Xms64m -Xmx2046m \
    -cp "$dest/config:$dest/lib/*" org.tio.sitexxx.all.Starter \
    > /opt/tanliao/server/logs/java.log 2>&1 < /dev/null 9>&- &
else
  cat > /opt/tanliao/server/run/http_server.py <<'PYTHON_SERVER'
#!/usr/bin/env python3
"""静态页面与同源 API 转发，仅代理固定的探聊后端。"""
import argparse
import http.client
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import urlsplit

HOP_HEADERS = {"connection", "keep-alive", "proxy-authenticate",
               "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade"}


def handler(directory, backend_host, backend_port):
    class Handler(SimpleHTTPRequestHandler):
        def __init__(self, *args, **kwargs):
            super().__init__(*args, directory=directory, **kwargs)

        def dispatch(self):
            path = urlsplit(self.path).path
            if path == "/mytio" or path.startswith("/mytio/"):
                self.proxy()
            elif self.command in ("GET", "HEAD"):
                if self.command == "HEAD":
                    super().do_HEAD()
                else:
                    super().do_GET()
            else:
                self.send_error(405)

        def proxy(self):
            connection = http.client.HTTPConnection(backend_host, backend_port, timeout=60)
            try:
                if self.headers.get("Transfer-Encoding"):
                    self.send_error(501, "Chunked request bodies are not supported")
                    return
                length = int(self.headers.get("Content-Length", "0"))
                if not 0 <= length <= 100 * 1024 * 1024:
                    self.send_error(413)
                    return
                body = self.rfile.read(length) if length else None
                blocked = HOP_HEADERS | {
                    item.strip().lower() for item in self.headers.get("Connection", "").split(",")
                }
                headers = {key: value for key, value in self.headers.items()
                           if key.lower() not in blocked | {"host", "content-length"}}
                headers["Host"] = self.headers.get("Host", backend_host)
                headers["X-Forwarded-For"] = self.client_address[0]
                headers["X-Forwarded-Proto"] = "http"
                connection.request(self.command, self.path, body=body, headers=headers)
                response = connection.getresponse()
                data = response.read()
                self.send_response(response.status, response.reason)
                blocked = HOP_HEADERS | {
                    item.strip().lower() for item in (response.getheader("Connection") or "").split(",")
                }
                for key, value in response.getheaders():
                    if key.lower() not in blocked | {"content-length"}:
                        self.send_header(key, value)
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                if self.command != "HEAD":
                    self.wfile.write(data)
            except (OSError, ValueError, http.client.HTTPException):
                self.send_error(502, "Backend unavailable")
            finally:
                connection.close()

        do_GET = do_HEAD = do_POST = do_PUT = do_DELETE = do_PATCH = do_OPTIONS = dispatch

    return Handler


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--port", type=int, default=21009)
    parser.add_argument("--directory", required=True)
    parser.add_argument("--backend-host", default="127.0.0.1")
    parser.add_argument("--backend-port", type=int, default=21008)
    args = parser.parse_args()
    ThreadingHTTPServer(("0.0.0.0", args.port),
                        handler(args.directory, args.backend_host, args.backend_port)).serve_forever()
PYTHON_SERVER
  nohup python3 /opt/tanliao/server/run/http_server.py --port "$port" --directory "$dest" --backend-port "$backend_port" \
    > /opt/tanliao/server/logs/html.log 2>&1 < /dev/null 9>&- &
fi
pid=$!
echo "$pid" > "$pid_file"
for i in {1..60}; do
  if ! kill -0 "$pid" 2>/dev/null; then echo "进程退出，请查看 /opt/tanliao/server/logs/$kind.log" >&2; exit 1; fi
  if ss -ltnpH "sport = :$port" | grep -q "pid=$pid,"; then
    ready=1
    if [[ "$kind" == java ]]; then
      for check_port in "$ws_port" "$app_port"; do
        if ! ss -ltnpH "sport = :$check_port" | grep -q "pid=$pid,"; then ready=0; fi
      done
    fi
    if (( ready )); then
      echo "部署成功：${dest}，端口 ${port}，PID ${pid}"
      exit 0
    fi
  fi
  sleep 1
done
echo "启动超时，请查看 /opt/tanliao/server/logs/$kind.log" >&2
exit 1
REMOTE
