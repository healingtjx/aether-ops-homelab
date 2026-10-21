# aether-ops-homelab

面向家庭服务器的 Aether Ops 运维管理项目。

基于 Go 和 PostgreSQL，提供项目、应用、构建、发布脚本及 Docker 部署管理。

配置文件 `config.yaml` 纳入版本控制，包含当前环境的数据库连接信息和密码。MCP 凭据和 `_run/` 下的本地文件不纳入版本控制。

本地已编译二进制可直接启动：

```bash
./aether_ops
```

其他环境启动前请按需修改 `config.yaml` 中的数据库连接、端口和工作目录。环境变量（如 `DB_PASSWORD`）仍可覆盖文件配置。

## ECR 镜像清理

服务每天按运行环境时区在凌晨 3 点清理启用的镜像仓库，每个 repository 保留排序最新的 3 个 tag，启动时不立即执行清理。同一进程内已有清理任务运行时跳过重复触发。可通过 `artifact cleanup:` 日志查看执行结果。沿用现有时间排序：优先使用产物的 PushedAt，缺失时依次使用同步时间、创建时间；当前通用 Registry 同步可能取镜像构建时间，并非 ECR 实际推送时间。同步失败的仓库跳过清理，删除失败记录在操作日志中。

AWS ECR 的自动清理与页面手动删除均通过服务器 AWS CLI 调用 `ecr batch-delete-image`，只删除目标 tag，不按 digest 删除其他标签。区域和账户 ID 从私有 ECR 标准域名解析，支持 `amazonaws.com` 和 `amazonaws.com.cn`。

服务器须安装 AWS CLI，并为运行服务的账户配置 AWS 身份，授予目标仓库的 `ecr:BatchDeleteImage` 权限及现有同步所需读取权限。即使平台配置了 Registry 登录密码，删除仍使用服务器 AWS 身份；跨账户仓库还需目标仓库授权。删除操作最长等待 30 秒，目标 tag 已不存在视为成功。

清理策略暂不提供页面配置，不检查镜像是否正在部署使用，也不清理无 tag 镜像。ECR 仓库应填写需要同步的 repository 列表。
