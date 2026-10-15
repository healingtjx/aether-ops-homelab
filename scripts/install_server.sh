#!/usr/bin/env bash
set -euo pipefail

APP_HOME="${APP_HOME:-/opt/aether}"
PACKAGE_FILE="${1:-}"

usage() {
	cat <<EOF
用法: $0 [部署包路径]

默认行为:
  在 /opt/aether 下查找 aether_ops-*.tar.gz 并解压到 /opt/aether

环境变量:
  APP_HOME  应用目录，默认 /opt/aether
EOF
}

if [ "${PACKAGE_FILE}" = "-h" ] || [ "${PACKAGE_FILE}" = "--help" ]; then
	usage
	exit 0
fi

mkdir -p "${APP_HOME}"
cd "${APP_HOME}"

if [ -z "${PACKAGE_FILE}" ]; then
	PACKAGE_FILE="$(ls -t aether_ops-*.tar.gz 2>/dev/null | head -n 1 || true)"
fi

if [ -z "${PACKAGE_FILE}" ]; then
	echo "未找到部署包: ${APP_HOME}/aether_ops-*.tar.gz" >&2
	exit 1
fi

if [ ! -f "${PACKAGE_FILE}" ]; then
	echo "部署包不存在: ${PACKAGE_FILE}" >&2
	exit 1
fi

echo "解压部署包: ${PACKAGE_FILE}"
tar -xzf "${PACKAGE_FILE}" --strip-components=1

chmod +x start_server.sh 2>/dev/null || true
chmod +x bin/aether_ops 2>/dev/null || true

mkdir -p "${APP_HOME}/work/applications" "${APP_HOME}/work/ci"

if [ ! -f config.yaml ] && [ -f config.yaml.example ]; then
	cp config.yaml.example config.yaml
	echo "已生成配置文件: ${APP_HOME}/config.yaml"
	echo "请先修改数据库配置: vi ${APP_HOME}/config.yaml"
fi

echo "当前目录文件:"
ls -la

echo ""
echo "下一步:"
echo "  vi ${APP_HOME}/config.yaml"
echo "  ./start_server.sh start"
