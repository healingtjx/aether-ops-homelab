# 由 deploy.sh 通过 SSH 发送给目标服务器的 Bash 执行。
set -euo pipefail
stage='检查目标环境'
tmp=''
cleanup() {
  code=$?
  trap - EXIT
  [[ -z $tmp ]] || rm -f -- "$tmp"
  if (( code != 0 )); then printf '失败阶段: %s (exit=%s)\n' "$stage" "$code" >&2; fi
  exit "$code"
}
trap cleanup EXIT
for command in aws docker awk mktemp cp mv; do command -v "$command" >/dev/null; done
cd -- "$remote_dir"
[[ -f $compose_file && ! -L $compose_file ]] || { echo 'Compose 必须是普通文件，不能是符号链接' >&2; exit 1; }
stage='读取并修改 Compose 镜像'
umask 077
tmp=$(mktemp "./.${compose_file}.aether.XXXXXXXX")
cp -p -- "$compose_file" "$tmp"
export AETHER_SERVICE="$compose_service" AETHER_IMAGE="$image_ref"
awk "$image_filter" "$compose_file" > "$tmp"
stage='重新登录 ECR'
aws_args=(ecr get-login-password --region "$region")
[[ -z $profile ]] || aws_args+=(--profile "$profile")
aws "${aws_args[@]}" | docker login "$registry_host" --username AWS --password-stdin
stage='备份并替换 Compose'
backup="${compose_file}.aether.bak.${tmp##*.}"
cp -p -- "$compose_file" "$backup"
printf 'Compose 备份: %s/%s\n' "$remote_dir" "$backup"
mv -- "$tmp" "$compose_file"
tmp=''
stage='执行 Docker Compose 发布'
docker compose -f "$compose_file" up -d "$compose_service"
