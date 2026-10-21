#!/usr/bin/env bash
# Linux 直接部署：Java + Python 静态服务，不使用镜像。
set -Eeuo pipefail
: "${MODULE_NAME:?缺少 MODULE_NAME}"
: "${REMOTE_HOST:?缺少 REMOTE_HOST}"
REMOTE_USER="${REMOTE_USER:-tjx}"
REMOTE_PORT="${REMOTE_PORT:-22}"
case "$MODULE_NAME" in
  tio-mg-all) kind=java; default_port=21006 ;;
  pc-html) kind=html; default_port=21007 ;;
  *) echo '不支持的模块' >&2; exit 1 ;;
esac
DEPLOY_DIR="${DEPLOY_DIR:-/opt/tanliao/admin/$kind}"
SERVER_PORT="${SERVER_PORT:-$default_port}"
[[ "$DEPLOY_DIR" == "/opt/tanliao/admin/$kind" ]] || { echo '部署目录不正确' >&2; exit 1; }
[[ "$REMOTE_HOST" =~ ^[a-zA-Z0-9][a-zA-Z0-9.-]*$ ]]
[[ "$REMOTE_USER" =~ ^[a-zA-Z_][a-zA-Z0-9_-]*$ ]]
for port in "$SERVER_PORT" "$REMOTE_PORT"; do
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
ssh_run "bash -s -- '$kind'" <<'CHECK'
set -Eeuo pipefail
for cmd in flock ss sha256sum tar; do command -v "$cmd" >/dev/null; done
if [[ "$1" == java ]]; then
  java -version 2>&1 | grep 'version "1\.8\.' >/dev/null || { echo '目标服务器需要 Java 8' >&2; exit 1; }
else
  python3 --version
fi
CHECK
# 后端 Maven reactor 共享目录，避免同时构建污染产物。
mkdir .tanliao-build.lock 2>/dev/null || { echo '已有构建任务，或遗留 .tanliao-build.lock，请检查' >&2; exit 1; }
local_stage=$(mktemp -d)
remote_stage=''
cleanup() {
  rc=$?
  trap - EXIT
  if [[ -n "$remote_stage" ]]; then ssh_run "rm -f '$remote_stage/package.tar.gz'; rmdir '$remote_stage'" || true; fi
  rm -rf "$local_stage"
  rmdir .tanliao-build.lock
  exit "$rc"
}
trap cleanup EXIT
mkdir "$local_stage/package"
if [[ "$kind" == java ]]; then
  export JAVA_HOME="${JDK_HOME:-${JAVA_HOME:-/usr/lib/jvm/java-8-openjdk-amd64}}"
  export PATH="$JAVA_HOME/bin:$PATH"
  java -version 2>&1 | grep 'version "1\.8\.' >/dev/null
  mvn -B -f parent/pom.xml clean install -P linux -DskipTests
  mvn -B -f all/pom.xml clean install -P linux -DskipTests
  test -d all/target/tio-mg-all/lib
  test -f all/target/tio-mg-all/config/app.properties
  cp -R all/target/tio-mg-all/lib all/target/tio-mg-all/config "$local_stage/package/"
  # 使用仓库 dev 配置，不做 Docker host 替换。
  config="$local_stage/package/config/app.properties"
  sed -E "s/^tio\.env[[:space:]]*=.*/tio.env=dev/; s/^http\.api\.port[[:space:]]*=.*/http.api.port = $SERVER_PORT/" "$config" > "$config.tmp"
  mv "$config.tmp" "$config"
  grep -Eq "^http\.api\.port[[:space:]]*=[[:space:]]*$SERVER_PORT$" "$config"
else
  API_BASE_URL="${API_BASE_URL:-/tioadmin}"
  [[ "$API_BASE_URL" == /tioadmin ]]
  (
    cd pc-html
    node_major=$(node -p 'process.versions.node.split(".")[0]')
    if (( node_major >= 17 )); then export NODE_OPTIONS="${NODE_OPTIONS:-} --openssl-legacy-provider"; fi
    if [[ -f package-lock.json ]]; then npm ci; else npm install; fi
    VUE_APP_apiCtx="$API_BASE_URL" npm run build
  )
  test -s pc-html/dist/index.html
  cp -R pc-html/dist/. "$local_stage/package/"
fi
COPYFILE_DISABLE=1 tar -czf "$local_stage/package.tar.gz" -C "$local_stage/package" .
checksum=$(shasum -a 256 "$local_stage/package.tar.gz" | awk '{print $1}')
remote_stage=$(ssh_run 'mktemp -d /tmp/tanliao-admin.XXXXXXXX')
[[ "$remote_stage" =~ ^/tmp/tanliao-admin\.[a-zA-Z0-9]+$ ]] || { remote_stage=''; echo '远端临时目录不正确' >&2; exit 1; }
"${auth[@]}" scp "${ssh_opts[@]}" -P "$REMOTE_PORT" "$local_stage/package.tar.gz" "$REMOTE_USER@$REMOTE_HOST:$remote_stage/package.tar.gz"
ssh_run "bash -s -- '$kind' '$DEPLOY_DIR' '$SERVER_PORT' '$remote_stage' '$checksum'" <<'REMOTE'
set -Eeuo pipefail
kind=$1
dest=$2
port=$3
stage=$4
checksum=$5
[[ "$kind" == java || "$kind" == html ]]
[[ "$dest" == "/opt/tanliao/admin/$kind" ]]
[[ ! -L "$dest" ]]
mkdir -p "$dest" /opt/tanliao/admin/run /opt/tanliao/admin/logs
exec 9>"/opt/tanliao/admin/run/$kind.lock"
flock -n 9 || { echo '已有部署任务' >&2; exit 1; }
printf '%s  %s\n' "$checksum" "$stage/package.tar.gz" | sha256sum -c -
pid_file="/opt/tanliao/admin/run/$kind.pid"
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
if ss -ltnH "sport = :$port" | grep -q .; then echo "端口 $port 被其他进程占用" >&2; exit 1; fi
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
    -cp "$dest/config:$dest/lib/*" org.tio.mg.all.Starter \
    > /opt/tanliao/admin/logs/java.log 2>&1 < /dev/null 9>&- &
else
  cat > /opt/tanliao/admin/run/http_server.py <<'PYTHON_SERVER'
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
            if path == "/tioadmin" or path.startswith("/tioadmin/"):
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
    parser.add_argument("--port", type=int, default=21007)
    parser.add_argument("--directory", required=True)
    parser.add_argument("--backend-host", default="127.0.0.1")
    parser.add_argument("--backend-port", type=int, default=21006)
    args = parser.parse_args()
    ThreadingHTTPServer(("0.0.0.0", args.port),
                        handler(args.directory, args.backend_host, args.backend_port)).serve_forever()
PYTHON_SERVER
  nohup python3 /opt/tanliao/admin/run/http_server.py --port "$port" --directory "$dest" \
    > /opt/tanliao/admin/logs/html.log 2>&1 < /dev/null 9>&- &
fi
pid=$!
echo "$pid" > "$pid_file"
for i in {1..60}; do
  if ! kill -0 "$pid" 2>/dev/null; then echo "进程退出，请查看 /opt/tanliao/admin/logs/$kind.log" >&2; exit 1; fi
  if ss -ltnpH "sport = :$port" | grep -q "pid=$pid,"; then
    echo "部署成功：${dest}，端口 ${port}，PID ${pid}"
    exit 0
  fi
  sleep 1
done
echo "启动超时，请查看 /opt/tanliao/admin/logs/$kind.log" >&2
exit 1
REMOTE
