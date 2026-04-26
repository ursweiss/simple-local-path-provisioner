# Copilot Instructions

## Coding Standards

- **Go version**: 1.26.2+
- **Formatting**: use `gofmt` / `goimports`; one blank line between top-level declarations; no trailing whitespace
- **Linting**: all code must pass `golangci-lint run ./...` with the project `.golangci.yml` config
- **Security**: never disable `gosec` findings without a justifying comment; validate all user-supplied paths against the base path to prevent traversal

## Testing

### Running Tests

| Command | Purpose |
|---|---|
| `make test` | Run all unit tests (`go test ./...`) |
| `make lint` | Run linter (`golangci-lint run ./...`) |
| `make vuln` | Run vulnerability scan (`govulncheck ./...`) |
| `make helm-lint` | Lint the Helm chart |
| `make pre-commit` | Run all pre-commit hooks against all files |
| `make check` | **Run all of the above** — the single quality gate |

### Mandatory Rule

**After every code change, run `make check` and verify all checks pass before considering the change complete.**
Do not skip this step. If any check fails, fix the issue before proceeding.

### Testing Conventions

- Use the standard `testing` package with **table-driven tests**
- Test files live next to the code they test (e.g. `pkg/driver/pathutil_test.go`)
- Use `package driver` (same-package tests), not `package driver_test`, because key types (`identityServer`, `controllerServer`, `sanitizeComponent`, `validateCapabilities`) are unexported
- Use `t.TempDir()` for any test that touches the filesystem — never create or clean up temp dirs manually
- Do not add error handling for scenarios that cannot happen (e.g. do not check errors from `t.TempDir()`)

### Scope Exclusions

- **`pkg/driver/node.go`**: no unit tests — requires real Linux bind mount syscalls (`unix.Mount`). Testing this file requires root privileges and a real filesystem, which is not feasible in standard test environments.

## Project Structure

```
cmd/driver/main.go          Entry point
pkg/driver/                  CSI driver implementation
  config.go                  Config struct
  controller.go              CreateVolume, DeleteVolume, ControllerPublish/Unpublish
  identity.go                GetPluginInfo, GetPluginCapabilities, Probe
  meta.go                    MetaStore — per-volume metadata persistence
  node.go                    NodePublish/Unpublish (bind mounts, out of test scope)
  pathutil.go                Path sanitization, derivation, validation
  server.go                  gRPC server wiring
deploy/helm/                 Helm chart
```
