#!/usr/bin/env bash
set -euo pipefail

APP_NAME="aether_ops"
APP_HOME="${APP_HOME:-/opt/aether}"
CONFIG_FILE="${AETHER_CONFIG:-${APP_HOME}/config.yaml}"
PID_FILE="${PID_FILE:-${APP_HOME}/${APP_NAME}.pid}"
LOG_FILE="${LOG_FILE:-${APP_HOME}/${APP_NAME}.log}"
RUN_MODE="${1:-start}"

BIN_FILE="${BIN_FILE:-${APP_HOME}/bin/${APP_NAME}}"

usage() {
	cat <<EOF
用法: $0 {start|stop|restart|status|foreground}

环境变量:
  APP_HOME       应用目录，默认 /opt/aether
  AETHER_CONFIG  配置文件路径，默认 /opt/aether/config.yaml
  BIN_FILE       二进制路径，默认 /opt/aether/bin/aether_ops；不存在时自动尝试 /opt/aether/aether_ops
  PID_FILE       PID 文件路径，默认 /opt/aether/aether_ops.pid
  LOG_FILE       日志文件路径，默认 /opt/aether/aether_ops.log
EOF
}

ensure_ready() {
	if [ ! -x "${BIN_FILE}" ] && [ -x "${APP_HOME}/${APP_NAME}" ]; then
		BIN_FILE="${APP_HOME}/${APP_NAME}"
	fi
	if [ ! -x "${BIN_FILE}" ]; then
		echo "二进制不存在或不可执行: ${BIN_FILE}" >&2
		echo "也未找到可执行文件: ${APP_HOME}/${APP_NAME}" >&2
		exit 1
	fi
	if [ ! -f "${CONFIG_FILE}" ]; then
		echo "配置文件不存在: ${CONFIG_FILE}" >&2
		echo "可先执行: cp ${APP_HOME}/config.yaml.example ${CONFIG_FILE}" >&2
		exit 1
	fi

	mkdir -p "${APP_HOME}/work/applications" "${APP_HOME}/work/ci"
	touch "${LOG_FILE}"
}

is_running() {
	if [ ! -f "${PID_FILE}" ]; then
		return 1
	fi
	local pid
	pid="$(cat "${PID_FILE}")"
	if [ -z "${pid}" ]; then
		return 1
	fi
	kill -0 "${pid}" >/dev/null 2>&1
}

start() {
	ensure_ready
	if is_running; then
		echo "${APP_NAME} 已运行，PID: $(cat "${PID_FILE}")"
		return 0
	fi

	cd "${APP_HOME}"

	# 加载 .env 文件（如果存在）
	if [ -f "${APP_HOME}/.env" ]; then
		set -a
		# shellcheck disable=SC1090
		source "${APP_HOME}/.env"
		set +a
	fi

	nohup env AETHER_CONFIG="${CONFIG_FILE}" "${BIN_FILE}" >>"${LOG_FILE}" 2>&1 &
	echo "$!" >"${PID_FILE}"
	sleep 1

	if is_running; then
		echo "${APP_NAME} 启动成功，PID: $(cat "${PID_FILE}")"
		echo "日志: ${LOG_FILE}"
	else
		echo "${APP_NAME} 启动失败，请查看日志: ${LOG_FILE}" >&2
		rm -f "${PID_FILE}"
		exit 1
	fi
}

stop() {
	if ! is_running; then
		echo "${APP_NAME} 未运行"
		rm -f "${PID_FILE}"
		return 0
	fi

	local pid
	pid="$(cat "${PID_FILE}")"
	kill "${pid}"

	for _ in $(seq 1 20); do
		if ! kill -0 "${pid}" >/dev/null 2>&1; then
			rm -f "${PID_FILE}"
			echo "${APP_NAME} 已停止"
			return 0
		fi
		sleep 1
	done

	echo "${APP_NAME} 停止超时，执行强制停止: ${pid}" >&2
	kill -9 "${pid}" >/dev/null 2>&1 || true
	rm -f "${PID_FILE}"
}

status() {
	if is_running; then
		echo "${APP_NAME} 运行中，PID: $(cat "${PID_FILE}")"
	else
		echo "${APP_NAME} 未运行"
	fi
}

foreground() {
	ensure_ready
	cd "${APP_HOME}"

	if [ -f "${APP_HOME}/.env" ]; then
		set -a
		# shellcheck disable=SC1090
		source "${APP_HOME}/.env"
		set +a
	fi

	exec env AETHER_CONFIG="${CONFIG_FILE}" "${BIN_FILE}"
}

case "${RUN_MODE}" in
	start)
		start
		;;
	stop)
		stop
		;;
	restart)
		stop
		start
		;;
	status)
		status
		;;
	foreground)
		foreground
		;;
	-h|--help|help)
		usage
		;;
	*)
		usage >&2
		exit 1
		;;
esac
