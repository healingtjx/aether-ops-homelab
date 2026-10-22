# 产物发布：按映射选择 JSON 脚本

发布映射中的「发布脚本」决定产物发布按钮执行哪套步骤。该入口不执行「发布脚本库」中的 Shell 记录，也不执行关联 CI 的构建脚本。

## 默认行为与文件位置

旧映射和新建映射默认选择 Docker Compose，继续使用原 `AETHER_DEPLOY_STEPS_FILE` 指向的 JSON（默认 Ops 工作目录下 `upload_remote_steps.json`）。文件不存在时使用 Go 内置的 Compose 更新、备份和启动流程。原文件存在时仍执行原 JSON 的步骤，是否备份取决于原 JSON 内容。

自定义脚本扫描 `deploy.scripts_dir` 指定的目录，默认是工作目录下 `deploy-scripts`，可通过 `AETHER_DEPLOY_SCRIPTS_DIR` 覆盖。生产建议 `/opt/aether/deploy-scripts`；本地开发使用 `devops/deploy-scripts`。每次加载页面/编辑表单扫描目录，每次发布重新读取所选文件。文件名是映射保存的标识，重命名需同步修改映射。文件不存在、内容错误或参数缺失时发布失败，不回退到默认流程。

## JSON 格式

```json
{
  "name": "自定义 Compose 发布",
  "registry_login": true,
  "defaults": {},
  "required_vars": ["REMOTE_DIR", "COMPOSE_FILE", "SERVICE_NAME"],
  "before_upload": [],
  "after_upload": [],
  "restart": [
    {"name": "查看服务状态", "commands": [
      "cd ${REMOTE_DIR}",
      "docker compose -f ${COMPOSE_FILE} ps ${SERVICE_NAME}"
    ]}
  ]
}
```

上面是格式示例，只查看状态，不更新镜像。

- 三阶段依次执行，每个步骤的命令以 `&&` 连接；步骤之间不保留 `cd` 或 Shell 变量，必要时每步重新设置。
- `registry_login` 缺省为 true：Ops 获取仓库凭据，并在本次 SSH 目标上执行 Docker 登录。false 完全跳过这一步，也不会在 Ops 调 AWS CLI。
- `defaults` 定义可编辑参数及其默认值；`required_vars` 指定必填参数，界面自动生成输入框。
- 系统提供 `IMAGE_REF`、`IMAGE_TAG`、`SERVICE_NAME`、`REMOTE_DIR`、`COMPOSE_FILE`、`REGISTRY_HOST`、`REGISTRY_USER`，不允许映射参数覆盖。IMAGE_REF/IMAGE_TAG 来自用户选中的产物。
- 占位符必须作为独立、不加引号的 shell 参数，如 `cd ${REMOTE_DIR}`。执行器负责 shell 转义；不要写 `cd "${REMOTE_DIR}"` 或拼接 `${IMAGE_REF}:tag`。需要拼接时先赋值，例如 `set -- ${REMOTE_DIR}`，再使用 shell 的 `"$1/subdir"`。
- 未声明且无值的占位符报错，不从 Ops 环境补值。新 JSON 不提供 REGISTRY_TOKEN；凭据由执行器管理。
- JSON 是可信的运维代码，只应允许可信运维人员修改目录。参数里不要存放认证信息；SSH 认证使用独立字段。

## SSH 连接

映射默认继承 CI 中的 `DEPLOY_SSH_*`；如果 CI 没有这些字段，保持原环境配置回退规则。

选择「独立配置」后，地址、端口、用户名、认证完全使用映射配置，不与 CI 混用。支持账户密码和 Ops 主机私钥文件路径，可填写私钥口令。密码/口令不回显，空白保留原值；切换认证方式移除另一种方式的凭据。独立 SSH 信息存储于数据库的 `deploy_ssh` JSONB 中，和现有 CI 凭据一样依赖数据库访问控制，不新增加密服务。

## tanliao 跳板机发布

使用 `devops/deploy-scripts/bastion-deploy.json`，调用链为：

`产物发布按钮 → 映射独立 SSH → 跳板机 deploy.sh → 目标服务器 → ECR 登录和 Compose 发布`

pc-html 映射填写：

| 字段 | 值 |
| --- | --- |
| 发布脚本 | 通用跳板机发布 |
| SSH 配置 | 独立配置 |
| 主机 / 端口 / 用户 | 43.213.226.16 / 22 / ubuntu |
| 认证 | 按实际账户密码或 Ops 上的私钥路径填写 |
| DEPLOY_PROJECT | tanliao |
| DEPLOY_ENV | prod |
| 服务名 | pc-html |
| BASTION_SCRIPT | /opt/bastion-deploy/deploy.sh |
| BASTION_CONFIG | /opt/bastion-deploy/deploy.conf |

远端目录和 Compose 文件由跳板机配置文件决定，映射中的这两个字段对该 JSON 不生效。跳板机下游服务器认证、ECR 仓库、目录等继续使用 `deploy.conf`。

JSON 传递完整 `--expected-image`，新版 deploy.sh 会在连接目标服务器之前核对配置生成的镜像是否等于所选产物。必须同步更新跳板机上的 deploy.sh，否则旧版会报未知参数；手动使用原参数调用仍兼容。

## 部署顺序

1. 对 Ops 数据库执行 `migrations/022_add_deploy_script_settings.sql`。迁移只新增字段，默认保留旧行为，可重复执行。程序不会自动执行迁移。
2. 更新跳板机 `/opt/bastion-deploy/deploy.sh`，其余脚本和配置沿用现有文件。
3. 构建并上传新版 Ops 程序，将 `devops/deploy-scripts/*.json` 放入 Ops 配置的脚本目录。`devops/upload_compiled.sh` 已支持上传此目录，可用 `LOCAL_DEPLOY_SCRIPTS` / `REMOTE_DEPLOY_SCRIPTS` 覆盖上传位置；自定义远端位置须与 Ops 配置一致。保留旧 `upload_remote_steps.json`。
4. 重启 Ops，在 pc-html 映射中选择跳板机 JSON，填参数和独立认证并保存。其他映射不变。
5. 先选择当前同版本产物验证调用链，查看日志中的脚本名、SSH 目标和跳板机输出。相同版本不保证重建容器；本功能不额外强制拉取镜像、执行健康检查或自动回滚。

本次开发未执行数据库迁移、服务器上传、映射切换或生产发布。回退单条映射时选择 Docker Compose 并恢复「继承 CI 配置」即可；回退程序可保留新增数据库列和 JSON 文件。
