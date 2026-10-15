# 数据库文档

本文档基于当前连接的 PostgreSQL 数据库 `aether_ops.public` 汇总，读取时间：2026-05-21。

当前数据库版本：PostgreSQL 9.5.18。

## 总览

### 核心关系

```text
projects 1 ── N applications 1 ── N ci_configs N ── 1 release_scripts
   │              │                    │
   │              │                    └── operation_logs.ci_config_id 可选关联
   │              └── N operation_logs
   └── N operation_logs

users 1 ── N sessions
users 1 ── N operation_logs（可选，删除用户后日志 user_id 置空）

global_tool_config 为单行全局配置表，固定 id = 1。
```

### 表清单

| 表名 | 当前行数 | 说明 |
|---|---:|---|
| `projects` | 1 | 项目表，应用的顶层分组 |
| `applications` | 1 | 应用表，记录 Git 仓库、默认分支和拉取状态 |
| `git_credentials` | 0 | Git 凭证旧表，当前代码基本不再使用，但真实库仍存在 |
| `users` | 1 | 登录用户 |
| `sessions` | 3 | 用户会话 |
| `release_scripts` | 1 | 发布脚本模板 |
| `ci_configs` | 10 | 应用、发布脚本、模块、分支的 CI 配置 |
| `operation_logs` | 37 | 应用拉取、CI 执行等操作日志 |
| `global_tool_config` | 1 | Git/Maven 工具路径全局配置 |

## 表结构

### `projects` 项目表

| 字段 | 类型 | 空值 | 默认值 | 说明 |
|---|---|---|---|---|
| `id` | `BIGSERIAL` | 否 | 自增 | 主键 |
| `name` | `VARCHAR(255)` | 否 |  | 项目名称，全局唯一 |
| `description` | `TEXT` | 是 |  | 项目描述 |
| `created_at` | `TIMESTAMPTZ` | 否 | `now()` | 创建时间 |
| `updated_at` | `TIMESTAMPTZ` | 否 | `now()` | 更新时间 |

约束：

| 名称 | 定义 |
|---|---|
| `projects_pkey` | `PRIMARY KEY (id)` |
| `projects_name_key` | `UNIQUE (name)` |

索引：

| 名称 | 字段 |
|---|---|
| `projects_pkey` | `id` |
| `projects_name_key` | `name` |

### `applications` 应用表

| 字段 | 类型 | 空值 | 默认值 | 说明 |
|---|---|---|---|---|
| `id` | `BIGSERIAL` | 否 | 自增 | 主键 |
| `project_id` | `BIGINT` | 否 |  | 所属项目 |
| `name` | `VARCHAR(255)` | 否 |  | 应用名称，同一项目内唯一 |
| `description` | `TEXT` | 是 |  | 应用描述 |
| `git_url` | `VARCHAR(1024)` | 否 |  | Git 仓库地址 |
| `default_branch` | `VARCHAR(255)` | 否 | `'main'` | 默认/最近选择的分支 |
| `git_credential_id` | `BIGINT` | 是 |  | 旧 Git 凭证引用，当前代码未使用 |
| `created_at` | `TIMESTAMPTZ` | 否 | `now()` | 创建时间 |
| `updated_at` | `TIMESTAMPTZ` | 否 | `now()` | 更新时间 |
| `last_pulled_at` | `TIMESTAMPTZ` | 是 |  | 最近拉取成功时间 |
| `last_pull_status` | `VARCHAR(50)` | 是 |  | 最近拉取状态 |
| `last_pull_message` | `TEXT` | 是 |  | 最近拉取消息 |

约束：

| 名称 | 定义 |
|---|---|
| `applications_pkey` | `PRIMARY KEY (id)` |
| `applications_project_id_name_key` | `UNIQUE (project_id, name)` |
| `applications_project_id_fkey` | `FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE` |
| `applications_git_credential_id_fkey` | `FOREIGN KEY (git_credential_id) REFERENCES git_credentials(id) ON DELETE SET NULL` |

索引：

| 名称 | 字段 |
|---|---|
| `applications_pkey` | `id` |
| `applications_project_id_name_key` | `project_id, name` |
| `idx_applications_project_id` | `project_id` |
| `idx_applications_git_credential_id` | `git_credential_id` |

### `git_credentials` Git 凭证旧表

该表在当前真实库仍存在，但迁移目录里有 `006_drop_git_credentials.sql` 用于删除它；当前应用代码也不再围绕该表建模。后续应决定保留还是清理，避免数据库和代码继续偏离。

| 字段 | 类型 | 空值 | 默认值 | 说明 |
|---|---|---|---|---|
| `id` | `BIGSERIAL` | 否 | 自增 | 主键 |
| `name` | `VARCHAR(255)` | 否 |  | 凭证名称 |
| `type` | `VARCHAR(50)` | 否 |  | 凭证类型：`ssh` 或 `https` |
| `username` | `VARCHAR(255)` | 是 |  | 用户名 |
| `password` | `TEXT` | 是 |  | HTTPS token/密码 |
| `ssh_private_key` | `TEXT` | 是 |  | SSH 私钥 |
| `ssh_passphrase` | `TEXT` | 是 |  | SSH 私钥密码 |
| `created_at` | `TIMESTAMPTZ` | 否 | `now()` | 创建时间 |
| `updated_at` | `TIMESTAMPTZ` | 否 | `now()` | 更新时间 |

约束：

| 名称 | 定义 |
|---|---|
| `git_credentials_pkey` | `PRIMARY KEY (id)` |
| `git_credentials_type_check` | `CHECK (type IN ('ssh', 'https'))` |

### `users` 用户表

| 字段 | 类型 | 空值 | 默认值 | 说明 |
|---|---|---|---|---|
| `id` | `BIGSERIAL` | 否 | 自增 | 主键 |
| `username` | `VARCHAR(255)` | 否 |  | 登录用户名，全局唯一 |
| `password_hash` | `VARCHAR(255)` | 否 |  | 密码哈希 |
| `created_at` | `TIMESTAMPTZ` | 否 | `now()` | 创建时间 |
| `updated_at` | `TIMESTAMPTZ` | 否 | `now()` | 更新时间 |

约束：

| 名称 | 定义 |
|---|---|
| `users_pkey` | `PRIMARY KEY (id)` |
| `users_username_key` | `UNIQUE (username)` |

### `sessions` 会话表

| 字段 | 类型 | 空值 | 默认值 | 说明 |
|---|---|---|---|---|
| `id` | `BIGSERIAL` | 否 | 自增 | 主键 |
| `token` | `VARCHAR(64)` | 否 |  | 会话 token，全局唯一 |
| `user_id` | `BIGINT` | 否 |  | 所属用户 |
| `created_at` | `TIMESTAMPTZ` | 否 | `now()` | 创建时间 |
| `expires_at` | `TIMESTAMPTZ` | 否 |  | 过期时间 |

约束：

| 名称 | 定义 |
|---|---|
| `sessions_pkey` | `PRIMARY KEY (id)` |
| `sessions_token_key` | `UNIQUE (token)` |
| `sessions_user_id_fkey` | `FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE` |

索引：

| 名称 | 字段 |
|---|---|
| `idx_sessions_token` | `token` |
| `idx_sessions_expires_at` | `expires_at` |

说明：`token` 已有唯一索引 `sessions_token_key`，`idx_sessions_token` 是重复普通索引，可考虑清理。

### `release_scripts` 发布脚本表

| 字段 | 类型 | 空值 | 默认值 | 说明 |
|---|---|---|---|---|
| `id` | `BIGSERIAL` | 否 | 自增 | 主键 |
| `name` | `VARCHAR(255)` | 否 |  | 脚本名称，全局唯一 |
| `description` | `TEXT` | 是 |  | 脚本描述 |
| `content` | `TEXT` | 否 |  | 脚本内容 |
| `default_variables` | `JSONB` | 否 | `'{}'::jsonb` | 默认变量配置 |
| `created_at` | `TIMESTAMPTZ` | 否 | `now()` | 创建时间 |
| `updated_at` | `TIMESTAMPTZ` | 否 | `now()` | 更新时间 |

约束：

| 名称 | 定义 |
|---|---|
| `release_scripts_pkey` | `PRIMARY KEY (id)` |
| `release_scripts_name_key` | `UNIQUE (name)` |

### `ci_configs` CI 配置表

| 字段 | 类型 | 空值 | 默认值 | 说明 |
|---|---|---|---|---|
| `id` | `BIGSERIAL` | 否 | 自增 | 主键 |
| `application_id` | `BIGINT` | 否 |  | 所属应用 |
| `release_script_id` | `BIGINT` | 否 |  | 使用的发布脚本 |
| `release_branch` | `VARCHAR(255)` | 否 | `'main'` | 发布分支 |
| `variables` | `JSONB` | 否 | `'{}'::jsonb` | 覆盖变量配置 |
| `last_run_status` | `VARCHAR(50)` | 是 |  | 最近运行状态 |
| `last_run_message` | `TEXT` | 是 |  | 最近运行消息 |
| `last_run_at` | `TIMESTAMPTZ` | 是 |  | 最近运行成功时间 |
| `created_at` | `TIMESTAMPTZ` | 否 | `now()` | 创建时间 |
| `updated_at` | `TIMESTAMPTZ` | 否 | `now()` | 更新时间 |
| `module_name` | `VARCHAR(255)` | 否 | `''` | 模块名称 |
| `module_path` | `VARCHAR(1024)` | 否 | `''` | 模块路径 |

约束：

| 名称 | 定义 |
|---|---|
| `ci_configs_pkey` | `PRIMARY KEY (id)` |
| `ci_configs_application_module_script_key` | `UNIQUE (application_id, module_name, release_script_id)` |
| `ci_configs_application_id_fkey` | `FOREIGN KEY (application_id) REFERENCES applications(id) ON DELETE CASCADE` |
| `ci_configs_release_script_id_fkey` | `FOREIGN KEY (release_script_id) REFERENCES release_scripts(id) ON DELETE RESTRICT` |

索引：

| 名称 | 字段 |
|---|---|
| `ci_configs_pkey` | `id` |
| `ci_configs_application_module_script_key` | `application_id, module_name, release_script_id` |
| `idx_ci_configs_application_id` | `application_id` |
| `idx_ci_configs_release_script_id` | `release_script_id` |
| `idx_ci_configs_application_script_module` | `application_id, release_script_id, module_name` |

说明：`ci_configs_application_module_script_key` 和 `idx_ci_configs_application_script_module` 都是同三列的唯一索引，只是列顺序不同。当前代码创建时按应用、发布脚本、模块判断重复，真实库同时存在两套唯一约束/索引，后续可统一。

### `operation_logs` 操作日志表

| 字段 | 类型 | 空值 | 默认值 | 说明 |
|---|---|---|---|---|
| `id` | `BIGSERIAL` | 否 | 自增 | 主键 |
| `application_id` | `BIGINT` | 否 |  | 应用 ID |
| `project_id` | `BIGINT` | 否 |  | 项目 ID |
| `operation_type` | `VARCHAR(50)` | 否 |  | 操作类型，如应用拉取、CI 执行 |
| `status` | `VARCHAR(50)` | 否 | `'running'` | 执行状态 |
| `message` | `TEXT` | 是 |  | 执行消息 |
| `started_at` | `TIMESTAMPTZ` | 否 | `now()` | 开始时间 |
| `finished_at` | `TIMESTAMPTZ` | 是 |  | 结束时间 |
| `user_id` | `BIGINT` | 是 |  | 操作用户 |
| `ci_config_id` | `BIGINT` | 是 |  | 关联 CI 配置 |
| `release_script_id` | `BIGINT` | 是 |  | 执行时关联的发布脚本 |
| `release_script_name` | `VARCHAR(255)` | 是 |  | 执行时发布脚本名称快照 |
| `release_branch` | `VARCHAR(255)` | 是 |  | 执行分支快照 |
| `module_name` | `VARCHAR(255)` | 是 |  | 模块名称快照 |
| `module_path` | `VARCHAR(1024)` | 是 |  | 模块路径快照 |

约束：

| 名称 | 定义 |
|---|---|
| `operation_logs_pkey` | `PRIMARY KEY (id)` |
| `operation_logs_application_id_fkey` | `FOREIGN KEY (application_id) REFERENCES applications(id) ON DELETE CASCADE` |
| `operation_logs_project_id_fkey` | `FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE` |
| `operation_logs_user_id_fkey` | `FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL` |
| `operation_logs_ci_config_id_fkey` | `FOREIGN KEY (ci_config_id) REFERENCES ci_configs(id) ON DELETE SET NULL` |
| `operation_logs_release_script_id_fkey` | `FOREIGN KEY (release_script_id) REFERENCES release_scripts(id) ON DELETE SET NULL` |

索引：

| 名称 | 字段 |
|---|---|
| `idx_operation_logs_app_id` | `application_id` |
| `idx_operation_logs_project_id` | `project_id` |
| `idx_operation_logs_user_id` | `user_id` |
| `idx_operation_logs_ci_config_id` | `ci_config_id` |
| `idx_operation_logs_started_at` | `started_at DESC` |
| `idx_operation_logs_release_branch` | `release_branch` |
| `idx_operation_logs_module_name` | `module_name` |

说明：日志表同时保存外键和脚本/分支/模块快照字段，目的是在 CI 配置或脚本被修改、删除后仍能展示历史执行上下文。

### `global_tool_config` 全局工具配置表

| 字段 | 类型 | 空值 | 默认值 | 说明 |
|---|---|---|---|---|
| `id` | `SMALLINT` | 否 | `1` | 主键，固定为 1 |
| `git_path` | `VARCHAR(1024)` | 否 | `'git'` | Git 可执行文件路径 |
| `maven_home_path` | `VARCHAR(1024)` | 否 | `''` | Maven Home 路径 |
| `maven_user_settings_file` | `VARCHAR(1024)` | 否 | `''` | Maven 用户 settings.xml |
| `maven_local_repository` | `VARCHAR(1024)` | 否 | `''` | Maven 本地仓库路径 |
| `npm_home_path` | `VARCHAR(1024)` | 否 | `''` | NPM Home 路径 |
| `npm_user_config_file` | `VARCHAR(1024)` | 否 | `''` | NPM 用户配置文件 |
| `npm_cache_directory` | `VARCHAR(1024)` | 否 | `''` | NPM 缓存目录 |
| `created_at` | `TIMESTAMPTZ` | 否 | `now()` | 创建时间 |
| `updated_at` | `TIMESTAMPTZ` | 否 | `now()` | 更新时间 |

约束：

| 名称 | 定义 |
|---|---|
| `global_tool_config_pkey` | `PRIMARY KEY (id)` |
| `global_tool_config_singleton` | `CHECK (id = 1)` |

## 迁移文件现状

| 文件 | 作用 |
|---|---|
| `001_create_projects.sql` | 创建项目表 |
| `002_create_git_credentials.sql` | 创建 Git 凭证表 |
| `003_create_applications.sql` | 创建应用表及应用索引 |
| `004_add_pull_fields_to_applications.sql` | 给应用表增加拉取状态字段 |
| `005_create_operation_logs.sql` | 创建操作日志表 |
| `006_create_users_and_sessions.sql` | 创建用户和会话表 |
| `006_drop_git_credentials.sql` | 删除 Git 凭证表及应用表引用 |
| `007_add_user_id_to_operation_logs.sql` | 操作日志增加用户引用 |
| `008_create_release_scripts_and_ci_configs.sql` | 创建发布脚本和 CI 配置表 |
| `009_add_release_branch_to_ci_configs.sql` | CI 配置增加发布分支 |
| `010_create_global_tool_config.sql` | 创建全局工具配置表 |
| `011_update_global_tool_config_maven_fields.sql` | 调整 Maven 相关配置字段 |
| `012_add_variables_to_release_scripts_and_ci_configs.sql` | 发布脚本和 CI 配置增加 JSONB 变量 |
| `013_add_modules_to_ci_configs.sql` | CI 配置增加模块字段并调整唯一性 |
| `014_add_ci_context_to_operation_logs.sql` | 操作日志增加 CI 上下文字段 |
| `015_add_npm_to_global_tool_config.sql` | 全局工具配置增加 NPM 配置字段 |

## 当前问题与整理建议

1. `006` 编号重复：`006_create_users_and_sessions.sql` 和 `006_drop_git_credentials.sql` 同时存在。建议重排或补一份明确的后续迁移，避免新环境按文件名执行时顺序不确定。
2. 真实库仍有 `git_credentials` 和 `applications.git_credential_id`，但迁移目录已有删除脚本，代码模型当前也未使用该能力。建议二选一：恢复凭证功能，或补齐并执行清理迁移。
3. 当前库没有发现迁移版本记录表，无法从数据库判断哪些迁移已执行。建议引入 `schema_migrations` 或迁移工具，至少记录文件名、checksum、执行时间。
4. `ci_configs` 存在两套三列唯一索引：`application_id, module_name, release_script_id` 和 `application_id, release_script_id, module_name`。如果业务只需要“同应用、同脚本、同模块唯一”，保留一套即可。
5. `sessions.token` 已有唯一索引，再建 `idx_sessions_token` 的收益较低，可评估删除普通索引。
6. `updated_at` 依赖应用层手动更新，没有数据库 trigger。若未来有多入口写入，建议统一用触发器维护。
