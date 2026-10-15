# Development and project layout

[Home](README.md) · [中文](../development.md)

Run commands from the repository root.

## Project layout

Source builds require Go 1.27.2 or newer. Container builds use the same version; the release workflow reads the requirement from the module manifest.

The top-level layout follows the relevant parts of [golang-standards/project-layout](https://github.com/golang-standards/project-layout). All tests live under [tests/](../../tests/).

| Path | Responsibility |
|---|---|
| [cmd/gemini-web2api/](../../cmd/gemini-web2api/) | Executable entry point |
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
| [configs/](../../configs/) | Configuration templates; runtime lookup paths remain unchanged |
| [build/package/](../../build/package/) | Container image definition |
| [deployments/](../../deployments/) | Compose configuration |
| [.github/workflows/](../../.github/workflows/) | Container and binary release workflows |

Run verification from the repository root:

On Windows, use `go build -o gemini-web2api.exe ./cmd/gemini-web2api` when you need an executable file. The all-package build check below does not leave an executable in the repository root.

```bash
go build ./...
go test ./... -count=1
go vet ./...
```

Each test package uses a temporary SQLite database and ignores production DSNs. The live upload probe requires explicit opt-in through its environment variable. See the [architecture notes](../architecture.md) for package responsibilities and migration details.

When upgrading an existing Compose deployment, retain its original project name with `-p` so the moved deployment file does not select a new volume prefix. Do not delete the original volume.

## Container verification with WSLC on Windows

If WSLC is installed, build and run the image without requiring a conventional WSL distribution:

```powershell
wslc build --progress plain -f "build/package/Dockerfile" -t "gemini-web2api-go:local" "."
wslc run --rm --publish "127.0.0.1:18083:8083" --env "ADMIN_TOKEN=local-verification-only" "gemini-web2api-go:local"
```

This uses only the test container's writable database and mounts no existing volume. Once ready, check `http://127.0.0.1:18083/` and the admin page. Stopping the container removes its temporary data through `--rm`.

An image build and a standalone container test do not verify Compose orchestration. Use Docker Compose v2 to check environment expansion, project naming and named-volume behavior.

---

Read more: [Deployment](deployment.md) · [API and models](api.md) · [Accounts and proxies](operations.md) · [Troubleshooting](troubleshooting.md) · [Development](development.md)
