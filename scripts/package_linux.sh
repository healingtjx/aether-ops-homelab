#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

export GOCACHE="${GOCACHE:-/private/tmp/aether_ops-go-build}"

APP_NAME="${APP_NAME:-aether_ops}"
VERSION="${1:-$(git describe --tags --always --dirty 2>/dev/null || date +%Y%m%d%H%M%S)}"
GOOS_TARGET="${GOOS_TARGET:-linux}"
GOARCH_TARGET="${GOARCH_TARGET:-amd64}"

DIST_DIR="${ROOT_DIR}/dist"
WORK_DIR="${DIST_DIR}/${APP_NAME}-${VERSION}-${GOOS_TARGET}-${GOARCH_TARGET}"
BIN_DIR="${WORK_DIR}/bin"
PACKAGE_NAME="${APP_NAME}-${VERSION}-${GOOS_TARGET}-${GOARCH_TARGET}.tar.gz"

rm -rf "${WORK_DIR}"
mkdir -p "${BIN_DIR}" "${WORK_DIR}/migrations" "${WORK_DIR}/doc/scripts" "${WORK_DIR}/systemd"

echo "==> testing"
go test ./...

echo "==> building ${GOOS_TARGET}/${GOARCH_TARGET}"
CGO_ENABLED=0 GOOS="${GOOS_TARGET}" GOARCH="${GOARCH_TARGET}" \
	go build -trimpath -ldflags="-s -w" -o "${BIN_DIR}/${APP_NAME}" .

cp migrations/*.sql "${WORK_DIR}/migrations/"
cp scripts/start_server.sh "${WORK_DIR}/start_server.sh"
cp scripts/install_server.sh "${WORK_DIR}/install_server.sh"
chmod +x "${WORK_DIR}/start_server.sh"
chmod +x "${WORK_DIR}/install_server.sh"
if [ -d doc/scripts ]; then
	cp -R doc/scripts/. "${WORK_DIR}/doc/scripts/"
fi

cat >"${WORK_DIR}/config.yaml.example" <<'YAML'
server:
  port: "8080"
  login_path: "/bs"

database:
  host: "127.0.0.1"
  port: 5432
  user: "postgres"
  password: "CHANGE_ME"
  dbname: "aether_ops"
  sslmode: "disable"
  max_conns: 20

work_dir:
  path: "/opt/aether/work"
  application_path: "/opt/aether/work/applications"
  ci_path: "/opt/aether/work/ci"
YAML

cat >"${WORK_DIR}/systemd/aether_ops.service" <<'UNIT'
[Unit]
Description=Aether Ops
After=network.target

[Service]
Type=simple
User=aether
Group=aether
WorkingDirectory=/opt/aether
Environment=AETHER_CONFIG=/opt/aether/config.yaml
ExecStart=/opt/aether/bin/aether_ops
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
UNIT

cat >"${WORK_DIR}/README.deploy.md" <<'MD'
# aether_ops 部署说明

## 1. 解压初始化

```bash
sudo mkdir -p /opt/aether
sudo cp aether_ops-*.tar.gz /opt/aether/
cd /opt/aether
sudo ./install_server.sh
```

## 2. 配置

```bash
cd /opt/aether
vi config.yaml
```

应用启动时读取 `AETHER_CONFIG` 指定的配置文件；未指定时读取当前目录的 `config.yaml` 或 `config.yml`。

## 3. 初始化数据库

按文件编号顺序执行 `migrations/*.sql`。示例：

```bash
for f in migrations/*.sql; do
  psql -h 127.0.0.1 -U postgres -d aether_ops -f "$f"
done
```

## 4. 创建登录用户

```bash
AETHER_CONFIG=/opt/aether/config.yaml /opt/aether/bin/aether_ops create-user admin 'your-password'
```

## 5. systemd 运行

```bash
sudo useradd --system --home /opt/aether --shell /usr/sbin/nologin aether
sudo chown -R aether:aether /opt/aether
sudo cp systemd/aether_ops.service /etc/systemd/system/aether_ops.service
sudo systemctl daemon-reload
sudo systemctl enable --now aether_ops
sudo systemctl status aether_ops
```

## 6. 不使用 systemd 时启动

```bash
./start_server.sh start
./start_server.sh status
./start_server.sh stop
```

查看日志：

```bash
tail -f /opt/aether/aether_ops.log
```
MD

tar -C "${DIST_DIR}" -czf "${DIST_DIR}/${PACKAGE_NAME}" "$(basename "${WORK_DIR}")"

echo "==> package created: ${DIST_DIR}/${PACKAGE_NAME}"
