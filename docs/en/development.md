# Development and project layout

[Home](README.md) · [中文](../development.md)

Run commands from the repository root.

## Project layout

Source builds require Go 1.27.2 or newer. Container builds use the same version; the release workflow reads the requirement from the module manifest.

The top-level layout follows the relevant parts of [golang-standards/project-layout](https://github.com/golang-standards/project-layout). All tests live under [tests/](../../tests/).

| Path | Responsibility |
|---|---|
| [cmd/gemrelay/](../../cmd/gemrelay/) | Executable entry point |
| [internal/app/](../../internal/app/) | Startup wiring and background scheduling |
| [internal/config/](../../internal/config/) | Startup settings, runtime snapshots and version |
| [internal/httpapi/](../../internal/httpapi/) | Routing, authentication, OpenAI/MCP adapters, admin and SSE |
| [internal/gateway/](../../internal/gateway/) | Generation orchestration, conversations, video jobs and tools |
| [internal/gemini/](../../internal/gemini/) | Upstream protocol, parsing, uploads, media and cookie rotation |
| [internal/pool/](../../internal/pool/) | Account/proxy selection, binding, capacity and health |
| [internal/cookies/](../../internal/cookies/) | Cookie normalization, extraction, identity and merge rules |
| [internal/storage/](../../internal/storage/) | Database lifecycle, dialects, transactions, statistics and persistence |
| [internal/transport/](../../internal/transport/) | Browser profiles, headers and HTTP clients |
| [tests/](../../tests/) | Grouped external tests, probes, isolated setup and fixtures |
| [web/admin/](../../web/admin/) | Admin assets and embedding declaration; bundled into the binary |
| [configs/](../../configs/) | Configuration templates; use an explicit path or a working-directory file |
| [build/package/](../../build/package/) | Container image definition |
| [deployments/](../../deployments/) | Compose configuration |
| [.github/workflows/](../../.github/workflows/) | Container and binary release workflows |

Run verification from the repository root:

On Windows, use `go build -o gemrelay.exe ./cmd/gemrelay` to create an executable. The all-package build command below checks compilation only.

```bash
go build ./...
go test ./... -count=1
go vet ./...
```

Each test package uses a temporary SQLite database and clears production DSN environment overrides. The live upload probe requires explicit opt-in through its environment variable. See the [architecture notes](../architecture.md) for package responsibilities and migration details.

## Continuous integration

PRs run Go formatting, `go vet`, tests and a build. Image and binary publishing reuse these checks; version tags must also match the code version.
Main-branch pushes publish images; version tags publish images and a Release. Manual runs on other refs retain build artifacts for 7 days as Actions artifacts.

## Container verification with WSLC on Windows

WSLC provides its own container runtime. Once installed, use it to build and run the image:

```powershell
wslc build --progress plain -f "build/package/Dockerfile" -t "gemrelay:local" "."
wslc run --rm --publish "127.0.0.1:18083:8083" --env "ADMIN_TOKEN=local-verification-only" "gemrelay:local"
```

This uses the test container's own database, isolated from existing volumes. Once ready, check `http://127.0.0.1:18083/` and the admin page. Stopping the container removes its temporary data through `--rm`.

Use WSLC to verify image builds and standalone containers, and Docker Compose v2 to verify environment expansion, project naming and named-volume behavior.

---

Read more: [Deployment](deployment.md) · [API and models](api.md) · [Accounts and proxies](operations.md) · [Troubleshooting](troubleshooting.md) · [Development](development.md)
