#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

REMOTE_HOST="${REMOTE_HOST:-192.168.5.4}"
REMOTE_USER="${REMOTE_USER:-tjx}"
REMOTE_PASSWORD="${REMOTE_PASSWORD:-Brtjx19981014@}"
REMOTE_DIR="${REMOTE_DIR:-/opt/aether}"
REMOTE_BIN="${REMOTE_BIN:-${REMOTE_DIR}/bin/aether_ops}"
LOCAL_BIN="${LOCAL_BIN:-${ROOT_DIR}/dist/aether_ops-linux-amd64}"
LOCAL_REMOTE_STEPS="${LOCAL_REMOTE_STEPS:-${ROOT_DIR}/devops/upload_remote_steps.json}"
REMOTE_STEPS="${REMOTE_STEPS:-${REMOTE_DIR}/upload_remote_steps.json}"
VERSION="${1:-}"

usage() {
	cat <<EOF
用法: $0 [版本号]

默认行为:
  1. 编译 Linux amd64 二进制文件
  2. 只上传并覆盖 ${REMOTE_USER}@${REMOTE_HOST}:${REMOTE_BIN}
  3. 不覆盖 config.yaml、work、migrations、脚本等其它文件

环境变量:
  REMOTE_HOST      目标服务器，默认 192.168.5.4
  REMOTE_USER      SSH 用户，默认 tjx
  REMOTE_PASSWORD  SSH 密码，默认 tangjiaxing
  REMOTE_DIR       远端应用目录，默认 /opt/aether
  REMOTE_BIN       远端程序路径，默认 /opt/aether/bin/aether_ops
  LOCAL_BIN        本地编译输出，默认 dist/aether_ops-linux-amd64
  LOCAL_REMOTE_STEPS  本地远端步骤 JSON，默认 devops/upload_remote_steps.json
  REMOTE_STEPS        远端步骤 JSON 路径，默认 /opt/aether/upload_remote_steps.json

示例:
  $0
  $0 v20260609
  REMOTE_BIN=/opt/aether/bin/aether_ops $0
EOF
}

if [ "${VERSION}" = "-h" ] || [ "${VERSION}" = "--help" ]; then
	usage
	exit 0
fi

mkdir -p "$(dirname "${LOCAL_BIN}")"

echo "==> testing"
go test ./...

echo "==> building linux/amd64"
if [ -n "${VERSION}" ]; then
	echo "版本标识: ${VERSION}"
fi
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
	go build -trimpath -ldflags="-s -w" -o "${LOCAL_BIN}" .

SSH_OPTS=(-o StrictHostKeyChecking=accept-new)
if command -v sshpass >/dev/null 2>&1; then
	SSH_CMD=(sshpass -p "${REMOTE_PASSWORD}" ssh "${SSH_OPTS[@]}")
	SCP_CMD=(sshpass -p "${REMOTE_PASSWORD}" scp "${SSH_OPTS[@]}")
else
	echo "未检测到 sshpass，将使用交互式 ssh/scp，请按提示输入密码。"
	SSH_CMD=(ssh "${SSH_OPTS[@]}")
	SCP_CMD=(scp "${SSH_OPTS[@]}")
fi

REMOTE_TMP="${REMOTE_BIN}.uploading"

echo "==> 确认远端程序目录: ${REMOTE_USER}@${REMOTE_HOST}:$(dirname "${REMOTE_BIN}")"
"${SSH_CMD[@]}" "${REMOTE_USER}@${REMOTE_HOST}" "mkdir -p '$(dirname "${REMOTE_BIN}")'"

echo "==> 上传程序到临时文件: ${REMOTE_TMP}"
"${SCP_CMD[@]}" "${LOCAL_BIN}" "${REMOTE_USER}@${REMOTE_HOST}:${REMOTE_TMP}"

echo "==> 覆盖远端程序: ${REMOTE_BIN}"
"${SSH_CMD[@]}" "${REMOTE_USER}@${REMOTE_HOST}" "chmod +x '${REMOTE_TMP}' && mv '${REMOTE_TMP}' '${REMOTE_BIN}'"

if [ -f "${LOCAL_REMOTE_STEPS}" ]; then
	echo "==> 上传远端步骤配置: ${REMOTE_STEPS}"
	"${SCP_CMD[@]}" "${LOCAL_REMOTE_STEPS}" "${REMOTE_USER}@${REMOTE_HOST}:${REMOTE_STEPS}"
fi

echo "==> 上传完成"
echo "远端程序: ${REMOTE_BIN}"
echo "远端步骤配置: ${REMOTE_STEPS}"
echo "重启命令:"
echo "  cd ${REMOTE_DIR} && ./start_server.sh restart"
