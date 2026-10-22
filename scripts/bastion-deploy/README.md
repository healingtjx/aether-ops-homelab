# 跳板机产物发布

调用方执行跳板机上的 `deploy.sh`，脚本通过 SSH 到目标服务器，重新登录 ECR、修改指定 Compose 服务的镜像并发布。可手动调用，也可由 Ops 发布映射选择跳板机 JSON 调用。

## 安装与配置

将本目录整体放到跳板机，例如 `/opt/bastion-deploy`。`remote.sh`、`replace-image.awk` 是入口依赖，不能只复制 `deploy.sh`。

- 跳板机：Bash 4+、OpenSSH 客户端、awk 及常用 Unix 命令；不需要 Python、yq、Docker 或 AWS CLI。
- 目标服务器：Bash、awk、AWS CLI、Docker Compose v2 和常用 Unix 命令；SSH 用户有部署目录写权限、Docker 权限及可用 AWS 身份。
- 使用 SSH 私钥认证，提前配置 known_hosts；启用严格主机检查，不进行交互式密码输入。带口令的私钥需提前加载到 ssh-agent。

```bash
cd /opt/bastion-deploy
cp deploy.conf.example deploy.conf
chmod 600 deploy.conf
# 编辑 deploy.conf，填写实际服务器及跳板机私钥路径。
```

配置是可信 Bash 文件，会被 `source` 执行，只允许运维修改。示例未包含私钥；`deploy.conf` 和 `keys/` 已被本目录 `.gitignore` 忽略。使用其他文件名保存真实配置时，应自行放在仓库之外。

每条配置使用 `项目/环境/服务` 作为关联数组键；新增项目或环境只需新增配置条目。

| 字段 | 含义 |
| --- | --- |
| `SSH_HOST` / `SSH_PORT` / `SSH_USER` | 目标服务器 SSH 地址、端口（默认 22）及用户 |
| `SSH_KEY` | 跳板机上的私钥文件路径 |
| `DEPLOY_DIR` | 目标服务器部署目录，必须为绝对路径 |
| `COMPOSE_FILE` | 部署目录内的 Compose 文件名，不支持子目录或符号链接 |
| `COMPOSE_SERVICE` | Compose 内的真实服务名 |
| `IMAGE_REPOSITORY` | 完整的私有 ECR 镜像仓库地址，不含 tag |
| `AWS_REGION` | ECR 区域 |
| `AWS_PROFILE` | 目标服务器上的 AWS profile；留空使用默认身份 |

除端口和 AWS profile 外，所有字段都必须填写。版本号通过命令传入。配置选择用的服务名可以与 Compose 服务名不同。

## 发布调用

```bash
bash /opt/bastion-deploy/deploy.sh \
  --config /opt/bastion-deploy/deploy.conf \
  --project tanliao --env prod --service pc-html --tag 26.1022.0
```

发布后端时将 `--service` 改为 `tio-mg-all`。macOS 默认 Bash 3.2 不支持此配置，需要显式使用安装的新版 Bash。

后续平台可以通过 SSH 调用跳板机入口（下列主机和用户名为占位）：

```bash
ssh -T deploy@bastion.example.com \
  'bash /opt/bastion-deploy/deploy.sh --config /opt/bastion-deploy/deploy.conf --project tanliao --env prod --service pc-html --tag 26.1022.0'
```

平台应采集标准输出、标准错误及退出码。镜像 tag 仅允许 Docker tag 字符，不接受任意命令。

## Ops 产物发布按钮接入

新版 Ops 在发布映射中选择 `bastion-deploy.json`，配置独立 SSH 和项目、环境参数即可使用本脚本。先执行数据库迁移并同步 Ops 程序、JSON 和新版 deploy.sh，具体见 [发布映射说明](../../doc/deploy-script-mappings.md)。仅更新本地代码不会改变线上按钮。

`--expected-image IMAGE` 为可选参数，平台调用时传入所选产物完整镜像地址；脚本核对配置生成的镜像一致后才连接目标服务器。原手动命令无需增加此参数。

## 修改范围与失败行为

脚本使用 awk 修改普通块式 Compose 中目标服务的直接 `image` 字段，保留其他行及镜像行末注释。支持常规带引号的服务名和镜像字符串，不支持目标服务的内联结构、锚点继承、变量形式的 image、多行 image、重复字段或制表符。无法明确定位时在登录和替换文件之前失败。它不是通用 YAML 编辑器；现有 Compose 应事先通过 Docker Compose 验证。

```yaml
services:
  pc-html:
    image: 319966842764.dkr.ecr.ap-northeast-1.amazonaws.com/tio-mg-pc-html:26.1018.0
    restart: unless-stopped
```

每次先生成临时文件，再在目标服务器执行 `aws ecr get-login-password | docker login --password-stdin`。登录成功后创建 `<Compose文件>.aether.bak.<随机后缀>` 备份，原子替换同目录 Compose 文件，并执行：

```bash
docker compose -f docker-compose.yaml up -d pc-html
```

命令失败即退出，不自动回滚。登录失败不会修改原文件；`up -d` 失败时新配置和原备份均保留，需人工处理。临时文件正常退出时清理，备份不自动删除。退出码为零只代表 Compose 命令成功，不代表业务健康检查通过。不强制拉取同 tag 镜像，建议每次使用新 tag。

同一 Compose 文件的发布需要调用方串行执行，包括前后端发布，避免并发覆盖。脚本不管理跨调用队列、超时或自动重试。

## 本地测试

```bash
bash scripts/bastion-deploy/tests/run.sh
```

测试需要 Bash 4+，使用临时目录和模拟 SSH/AWS/Docker，不连接远端服务器、不拉取镜像、不重启服务。
