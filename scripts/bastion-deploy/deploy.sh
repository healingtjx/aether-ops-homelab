#!/usr/bin/env bash
set -euo pipefail

fail() { printf '发布失败: %s\n' "$*" >&2; exit 1; }
usage() {
  echo '用法: bash deploy.sh --config FILE --project PROJECT --env ENV --service SERVICE --tag TAG [--expected-image IMAGE]'
}
if [[ ${1:-} == --help ]]; then usage; exit 0; fi
(( BASH_VERSINFO[0] >= 4 )) || fail '跳板机需要 Bash 4 或更新版本'
config='' project='' environment='' service='' tag='' expected_image=''
while (( $# )); do
  (( $# >= 2 )) || { usage >&2; exit 1; }
  case "$1" in
    --config) config=$2 ;;
    --project) project=$2 ;;
    --env) environment=$2 ;;
    --service) service=$2 ;;
    --tag) tag=$2 ;;
    --expected-image) expected_image=$2 ;;
    *) fail "未知参数: $1" ;;
  esac
  shift 2
done
[[ -n $config && -f $config ]] || fail '必须指定有效的 --config'
for value in "$project" "$environment" "$service"; do
  [[ $value =~ ^[a-zA-Z0-9][a-zA-Z0-9_.-]*$ ]] || fail '项目、环境和服务只能包含字母、数字、点、下划线和短横线'
done
[[ $tag =~ ^[a-zA-Z0-9_][a-zA-Z0-9_.-]{0,127}$ ]] || fail '镜像 tag 不合法'
for command in ssh awk; do command -v "$command" >/dev/null || fail "缺少命令: $command"; done
script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
declare -A SSH_HOST SSH_PORT SSH_USER SSH_KEY DEPLOY_DIR COMPOSE_FILE COMPOSE_SERVICE
declare -A IMAGE_REPOSITORY AWS_REGION AWS_PROFILE
# 配置是由运维管理的可信 Bash 文件。
source "$(cd -- "$(dirname -- "$config")" && pwd)/$(basename -- "$config")"
key="$project/$environment/$service"
host=${SSH_HOST[$key]:-}; port=${SSH_PORT[$key]:-22}
user=${SSH_USER[$key]:-}; identity=${SSH_KEY[$key]:-}
remote_dir=${DEPLOY_DIR[$key]:-}; compose_file=${COMPOSE_FILE[$key]:-}
compose_service=${COMPOSE_SERVICE[$key]:-}; repository=${IMAGE_REPOSITORY[$key]:-}
region=${AWS_REGION[$key]:-}; profile=${AWS_PROFILE[$key]:-}
[[ -n $host && -n $user && -n $identity && -n $remote_dir && -n $compose_file && -n $compose_service && -n $repository && -n $region ]] || fail "配置不存在或字段不完整: $key"
[[ $host =~ ^[a-zA-Z0-9][a-zA-Z0-9.:-]*$ && $user =~ ^[a-zA-Z0-9_][a-zA-Z0-9_.-]*$ ]] || fail 'SSH 主机或用户不合法'
[[ $port =~ ^[0-9]{1,5}$ ]] && (( 10#$port > 0 && 10#$port <= 65535 )) || fail 'SSH 端口不合法'
[[ -f $identity ]] || fail "SSH 私钥不存在: $identity"
[[ $remote_dir == /* && $remote_dir != *$'\n'* ]] || fail '部署目录必须为绝对路径'
[[ $compose_file =~ ^[a-zA-Z0-9_][a-zA-Z0-9_.-]*$ ]] || fail 'Compose 文件必须是目录内的文件名'
[[ $compose_service =~ ^[a-zA-Z0-9][a-zA-Z0-9_.-]*$ ]] || fail 'Compose 服务名不合法'
[[ $repository =~ ^[0-9]{12}\.dkr\.ecr\.[a-z0-9-]+\.amazonaws\.com(\.cn)?/[a-z0-9][a-z0-9_./-]*$ ]] || fail '镜像仓库必须为完整的私有 ECR repository 地址'
[[ $region =~ ^[a-z0-9-]+$ && $profile != *$'\n'* ]] || fail 'AWS 配置不合法'
image_ref="$repository:$tag"
[[ -z $expected_image || $expected_image == "$image_ref" ]] || fail "跳板机配置镜像与所选产物不一致: $image_ref"
registry_host=${repository%%/*}
image_filter=$(cat "$script_dir/replace-image.awk")
printf '发布目标: %s (%s@%s:%s)\n服务: %s\n新镜像: %s\n' "$key" "$user" "$host" "$port" "$compose_service" "$image_ref"
emit_script() {
  printf '%s\n' 'set -euo pipefail'
  for variable in remote_dir compose_file compose_service image_ref registry_host region profile image_filter; do
    printf '%s=%q\n' "$variable" "${!variable}"
  done
  cat "$script_dir/remote.sh"
}
if emit_script | ssh -T -o BatchMode=yes -o StrictHostKeyChecking=yes -o ConnectTimeout=15 \
  -p "$port" -i "$identity" "$user@$host" 'bash -s'; then
  echo '发布成功: Docker Compose 命令执行完成'
else
  fail '远端发布失败，请查看上方阶段日志；不自动回滚'
fi
