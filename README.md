# aether-ops-homelab

面向家庭服务器的 Aether Ops 运维管理项目。

基于 Go 和 PostgreSQL，提供项目、应用、构建、发布脚本及 Docker 部署管理。

配置文件 `config.yaml` 纳入版本控制，包含当前环境的数据库连接信息和密码。MCP 凭据和 `_run/` 下的本地文件不纳入版本控制。

本地已编译二进制可直接启动：

```bash
./aether_ops
```

其他环境启动前请按需修改 `config.yaml` 中的数据库连接、端口和工作目录。环境变量（如 `DB_PASSWORD`）仍可覆盖文件配置。
