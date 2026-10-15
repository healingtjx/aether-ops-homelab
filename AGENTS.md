# Repository Guidelines

## Project Structure & Module Organization

This is a Go web service in module `aether_ops`. The entry point is `main.go`, which loads configuration, connects to PostgreSQL, registers Chi routes, and embeds HTML templates.

- `internal/config`, `internal/db`: configuration loading and database connection setup.
- `internal/handlers`: HTTP handlers grouped by feature, such as projects, applications, auth, operation logs, release scripts, and CI configs.
- `internal/models`: database access and domain models.
- `internal/middleware`: request middleware, including authentication.
- `internal/gitops` and `internal/scriptexec`: repository operations and script execution helpers.
- `internal/templates` and `web/templates`: template parsing and HTML views.
- `migrations`: SQL schema migrations. Use numbered filenames such as `009_add_example_table.sql`.
- `doc`: project documentation, including database notes.

## Build, Test, and Development Commands

- `go run .`: start the service locally using `config.yaml`.
- `go run . create-user <username> <password>`: create a login user in the configured database.
- `go build ./...`: compile all packages.
- `go test ./...`: run all tests. This repository currently has no checked-in `_test.go` files, so add focused tests with new behavior.
- `gofmt -w <files>`: format modified Go files before committing.

Keep `config.example.yaml` updated when adding configuration fields. Do not commit real credentials in `config.yaml`.

## Coding Style & Naming Conventions

Follow standard Go style: tabs via `gofmt`, small packages, explicit error handling, and short receiver names. Use lower-case package names with no underscores. Name handler files by feature (`projects.go`, `ci_configs.go`) and keep model functions near the related model type. HTML templates are organized by feature directory under `web/templates/<feature>/`.

## Testing Guidelines

Use Go's standard `testing` package unless the repository adopts a specific test helper later. Name test files `*_test.go` and test functions `Test<Behavior>`. Prefer table-driven tests for model validation, config parsing, and handler behavior. For database behavior, isolate tests with a dedicated test database or transaction cleanup.

## Commit & Pull Request Guidelines

Recent commits use short subjects such as `feat : 登陆模块`, `feat : 项目拉取`, and `init`. Keep commits concise and action-oriented; prefer a consistent prefix such as `feat:`, `fix:`, `docs:`, or `test:` without mixing unrelated changes.

Pull requests should include a brief summary, database migration notes if applicable, configuration changes, and test results such as `go test ./...`. Include screenshots when changing `web/templates`.

## Agent-Specific Instructions

When responding to contributors in this repository, use Chinese by default unless the user explicitly requests another language. Keep replies concise, technical, and action-oriented.
