#!/usr/bin/env bash
set -euo pipefail
(( BASH_VERSINFO[0] >= 4 )) || { echo '测试需要 Bash 4+' >&2; exit 1; }
root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT
mkdir -p "$work/bin" "$work/target dir's" "$work/other"
export MOCK_DIR="$work" TEST_BASH="$BASH"
export PATH="$work/bin:$PATH"
touch "$work/key"
cat > "$work/bin/ssh" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$@" > "$MOCK_DIR/ssh.args"
[[ ${FAIL_SSH:-0} == 0 ]] || exit 255
exec "$TEST_BASH" -s
SH
cat > "$work/bin/aws" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$@" >> "$MOCK_DIR/aws.args"
[[ ${FAIL_AWS:-0} == 0 ]] || exit 41
printf 'test-secret-token'
SH
cat > "$work/bin/docker" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$@" >> "$MOCK_DIR/docker.args"
if [[ $1 == login ]]; then
  token=$(cat)
  [[ ${FAIL_LOGIN:-0} == 0 && $token == test-secret-token ]] || exit 42
else
  [[ ${FAIL_UP:-0} == 0 ]] || exit 43
fi
SH
chmod +x "$work/bin/ssh" "$work/bin/aws" "$work/bin/docker"
cat > "$work/config" <<'CONF'
declare -A SSH_HOST SSH_PORT SSH_USER SSH_KEY DEPLOY_DIR COMPOSE_FILE COMPOSE_SERVICE
declare -A IMAGE_REPOSITORY AWS_REGION AWS_PROFILE
for key in tanliao/prod/tio-mg-all tanliao/prod/pc-html another/prod/api; do
  SSH_HOST[$key]=example.com
  SSH_USER[$key]=deploy
  SSH_KEY[$key]="$MOCK_DIR/key"
  DEPLOY_DIR[$key]="$MOCK_DIR/target dir's"
  COMPOSE_FILE[$key]=docker-compose.yaml
  COMPOSE_SERVICE[$key]=${key##*/}
  IMAGE_REPOSITORY[$key]="319966842764.dkr.ecr.ap-northeast-1.amazonaws.com/${key##*/}"
  AWS_REGION[$key]=ap-northeast-1
done
IMAGE_REPOSITORY[tanliao/prod/pc-html]=319966842764.dkr.ecr.ap-northeast-1.amazonaws.com/tio-mg-pc-html
SSH_HOST[another/prod/api]=another.example.com
SSH_PORT[another/prod/api]=2222
DEPLOY_DIR[another/prod/api]="$MOCK_DIR/other"
AWS_PROFILE[another/prod/api]="deployment profile"
CONF
cat > "$work/original" <<'YAML'
# original comment
services:
  tio-mg-all:
    image: old/backend:v1
    environment:
      image: nested-not-a-service-image
  'pc-html':
    image: "old/frontend:v1" # keep this comment
    ports:
      - '80:80'
volumes:
  data: {}
YAML
compose="$work/target dir's/docker-compose.yaml"
invoke() { "$TEST_BASH" "$root/deploy.sh" --config "$work/config" --project tanliao --env prod --service pc-html --tag v2 "$@" > "$work/log" 2>&1; }
reset_case() {
  cp "$work/original" "$compose"
  : > "$work/aws.args"; : > "$work/docker.args"; : > "$work/ssh.args"
  unset FAIL_AWS FAIL_LOGIN FAIL_UP FAIL_SSH
}
assert_contains() { grep -F -- "$2" "$1" >/dev/null || { echo "缺少预期内容: $2 ($1)" >&2; exit 1; }; }
expect_failure() { if invoke "$@"; then echo '预期失败但成功' >&2; exit 1; fi; }

reset_case
invoke
sed 's|"old/frontend:v1"|319966842764.dkr.ecr.ap-northeast-1.amazonaws.com/tio-mg-pc-html:v2|' "$work/original" > "$work/expected"
cmp "$compose" "$work/expected"
backups=("$compose".aether.bak.*)
cmp "${backups[0]}" "$work/original"
assert_contains "$work/docker.args" 'up'
assert_contains "$work/ssh.args" 'StrictHostKeyChecking=yes'
if grep -F test-secret-token "$work/log"; then echo '令牌泄漏' >&2; exit 1; fi
echo 'PASS 前端精确修改、备份、路径转义和日志'

reset_case
invoke --service tio-mg-all
assert_contains "$compose" 'image: 319966842764.dkr.ecr.ap-northeast-1.amazonaws.com/tio-mg-all:v2'
assert_contains "$compose" 'image: "old/frontend:v1"'
echo 'PASS 后端映射'

printf 'services:\n  api:\n    image: old/api:v1\n' > "$work/other/docker-compose.yaml"
invoke --project another --service api
assert_contains "$work/ssh.args" 'deploy@another.example.com'
assert_contains "$work/ssh.args" '2222'
assert_contains "$work/aws.args" 'deployment profile'
assert_contains "$work/other/docker-compose.yaml" 'amazonaws.com/api:v2'
echo 'PASS 多项目、端口和 AWS profile'

for failure in FAIL_AWS FAIL_LOGIN FAIL_SSH; do
  reset_case
  export "$failure=1"
  expect_failure
  cmp "$compose" "$work/original"
  if grep -Fx up "$work/docker.args"; then echo '失败后仍执行发布' >&2; exit 1; fi
done
reset_case
export FAIL_UP=1
expect_failure
cmp "$compose" "$work/expected"
assert_contains "$work/log" '执行 Docker Compose 发布'
echo 'PASS SSH/登录失败不改文件、发布失败保留现场'

reset_case
expect_failure --tag '$(touch injected)'
expect_failure --project missing
[[ ! -s $work/ssh.args ]]
echo 'PASS 非法参数和缺失配置在 SSH 前终止'
reset_case
expect_failure --expected-image wrong/repository:v2
[[ ! -s $work/ssh.args ]]
cmp "$compose" "$work/original"
invoke --expected-image 319966842764.dkr.ecr.ap-northeast-1.amazonaws.com/tio-mg-pc-html:v2
cmp "$compose" "$work/expected"
echo 'PASS 所选产物与跳板机镜像一致性校验'


for fixture in missing duplicate inline inherited alias variable multiline tabs; do
  reset_case
  case "$fixture" in
    missing) printf 'services:\n  api:\n    image: old:v1\n' > "$compose" ;;
    duplicate) printf 'services:\n  pc-html:\n    image: old:v1\n    image: old:v2\n' > "$compose" ;;
    inline) printf 'services:\n  pc-html: {image: old:v1}\n' > "$compose" ;;
    inherited) printf 'services:\n  pc-html:\n    <<: *defaults\n    image: old:v1\n' > "$compose" ;;
    alias) printf 'services:\n  pc-html:\n    image: *image\n' > "$compose" ;;
    variable) printf 'services:\n  pc-html:\n    image: ${IMAGE}\n' > "$compose" ;;
    multiline) printf 'services:\n  pc-html:\n    image: >\n      old:v1\n' > "$compose" ;;
    tabs) printf 'services:\n\tpc-html:\n\t\timage: old:v1\n' > "$compose" ;;
  esac
  cp "$compose" "$work/before"
  expect_failure
  cmp "$compose" "$work/before"
  [[ ! -s $work/aws.args ]]
done
echo 'PASS 不支持的 Compose 结构不写回、不登录'
for source in "$root/deploy.sh" "$root/remote.sh" "$root/deploy.conf.example" "$root/tests/run.sh"; do
  "$TEST_BASH" -n "$source"
done
echo '全部测试通过'
