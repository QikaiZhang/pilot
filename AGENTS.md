# Repository Guidelines

## Project Structure & Module Organization

Pilot is a Go service module (`go.mod`, module name `Pilot`). The executable lives in `cmd/pilot/`. Keep application-only code in `internal/`: `controller/` owns HTTP handlers, `deps/` owns startup dependency checks, and future domain packages follow the responsibilities in `docs/development/README.md`. Reusable helpers belong in `pkg/`.

Deployment configuration is under `manifest/`, while design and staged-learning material lives in `docs/` and `stages/`. The untracked `源代码/` directory is reference material, not part of the root module's build or test scope.

## Build, Test, and Development Commands

- `make run` starts the HTTP service through `go run ./cmd/pilot`.
- `make build` produces `bin/pilot`.
- `make test` runs all root-module tests with `go test ./...`.
- `make tidy` synchronizes `go.mod` and `go.sum` after dependency changes.
- `make compose-up` starts local dependencies from `manifest/docker-compose.yml`; use `make compose-down` to stop them and `make compose-logs` to inspect them.
- `make health` checks the live and ready endpoints on port 8080.

Use Docker Compose before manually running a path that requires Redis, MySQL, or Elasticsearch. The service reads optional local settings from `.env`; never commit secrets, tokens, or real connection credentials.

## Coding Style & Naming Conventions

Write idiomatic Go and format every changed Go file with `gofmt`. Use tabs as Go tooling emits them, package names in lowercase, exported identifiers in `PascalCase`, and unexported identifiers in `camelCase`. Keep handlers thin, return wrapped errors with useful context, pass `context.Context` through request and dependency boundaries, and use table-driven tests where cases share behavior. Preserve the existing JSON response helpers rather than duplicating response encoding.

## Testing Guidelines

Place tests beside their package as `*_test.go`; use descriptive names such as `TestLoad_UsesEnvironmentOverride`. Cover normal behavior, invalid input, dependency failures, and health/readiness status changes. Run `make test` before submitting. For changes that touch external integrations, also start Compose and run `make health`; document any integration test that cannot run locally.

## Commit & Pull Request Guidelines

Recent history uses concise Conventional Commit-style subjects, especially `feat: ...`; continue with prefixes such as `feat:`, `fix:`, `test:`, and `docs:`. Keep each commit focused. Pull requests should explain the affected request/data flow, list validation commands, link the relevant stage or issue, and include API examples or screenshots when behavior visible to users changes. Update the appropriate `docs/` or `stages/` material when advancing a learning stage.
