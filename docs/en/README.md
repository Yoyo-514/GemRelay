# GemRelay

A Go service that exposes the Gemini web app through an OpenAI-compatible API, with account and proxy pools and an admin panel.
Based on [zexadev/gemini-web2api-go](https://github.com/zexadev/gemini-web2api-go), this version adds a modular layout, strict model checks, account quota queries and a mobile admin interface.

<img src="../banner.svg" alt="GemRelay: Gemini web to OpenAI-compatible API" width="100%">

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](../../LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.27.2%2B-00ADD8.svg)](../../go.mod)

[中文](../../README.md) | English

The service accesses Gemini through its web protocol, using anonymous requests or Google account cookies. Two text models work anonymously; other models and media features depend on account access and web-app quotas.

## Features

**API**
- OpenAI-compatible: `/v1/chat/completions`, `/v1/models`, `/v1/responses`
- `/v1/videos`: OpenAI (Sora)-shaped async video generation — `POST` to create, `GET /v1/videos/{id}` to poll, `GET /v1/videos/{id}/content` to download the MP4 (needs a signed-in Pro cookie)
- Incremental text streaming as upstream response frames arrive
- Bearer token / `x-api-key` auth; the key can be rotated from the panel
- `usage` estimated with tiktoken; `reasoning_tokens` and `completion_tokens` counted separately

**Models**
- `gemini-3.6-flash` and `gemini-3.5-flash-lite` work anonymously, web search included
- `gemini-3.1-pro` needs account access; reasoning is returned when upstream supplies it
- Text models offer extended-thinking variants with a cookie (`reasoning_content`)
- Records the actual model reported by upstream to help identify downgrades

**Accounts and exits**
- Configurable browser TLS profiles for direct connections, Chrome 146 by default; proxies use Go's standard HTTP client
- Per-exit-IP limits: concurrency / RPM / RPH
- Proxy pool: runtime CRUD, failure circuit breaker, rotation — each proxy is its
  own rate-limit slot
- Cookie pool: several Google accounts rotated least-recently-used first, auto-refreshed and kept alive, each account pinned to its own exit

**Operations**
- Single binary, cross-compiled for 6 platforms; container image built on distroless
- SQLite persistence: 30 days of per-request detail plus permanent aggregates (optional MySQL / PostgreSQL via `SQL_DSN`)
- Admin panel: overview / requests / proxy pool / cookie pool / settings, with
  runtime settings on the panel; restarting applies timeout changes to existing clients
- Request records contain metadata only: length, latency, model and status

## Quick start

### Docker

On a Linux deployment machine, run the published image, replacing `your-admin-token` with your own admin token:

```bash
docker run -d --name gemrelay \
  -p 0.0.0.0:8083:8083 \
  -v "gemrelay-data:/data" \
  ghcr.io/yoyo-514/gemrelay:latest \
  --db "/data/gemrelay.db" --port 8083 --admin-token your-admin-token
```

The image supports Linux amd64 / arm64 and stores data in the `gemrelay-data` volume. The `latest` tag tracks main-branch builds.

### Binary

Download the executable for your system and architecture from the [latest release](https://github.com/Yoyo-514/GemRelay/releases/latest), then rename it to `gemrelay` (`gemrelay.exe` on Windows). On Linux / macOS, run these commands from the program's directory:

```bash
chmod +x "./gemrelay"
"./gemrelay" --port 8083 --admin-token your-admin-token
```

The binary stores data in `./data/gemrelay.db` relative to its working directory. See [Deployment and configuration](deployment.md) for Windows, macOS and Compose instructions.

### Connect a client

The service listens on `0.0.0.0:8083` by default. Replace `SERVER_IP` below with the server's public or LAN IP, and allow TCP port 8083 in the host firewall and cloud security group.

Open `http://SERVER_IP:8083/admin`, sign in with the admin token, and find the API key generated on first boot on the Settings page.

Make a request:

```bash
curl "http://SERVER_IP:8083/v1/chat/completions" \
  -H "Authorization: Bearer sk-gemrelay-..." \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-3.6-flash",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

Set the client's base URL to `http://SERVER_IP:8083/v1`, use the key from the panel, and start with `gemini-3.6-flash`.

See [Deployment and configuration](deployment.md) for databases, startup settings and source builds.

## Documentation

| Task | Guide |
|---|---|
| Deploy, choose a database, set configuration | [Deployment and configuration](deployment.md) |
| Connect clients, choose models, use MCP, check protocol support | [API, models and clients](api.md) |
| Import cookies, inspect quota, manage proxies and rate limits | [Admin panel, accounts and proxies](operations.md) |
| Diagnose errors, check limits and common questions | [Limitations and troubleshooting](troubleshooting.md) |
| Build, test and understand the directory layout | [Development and project layout](development.md) · [Architecture notes (Chinese)](../architecture.md) |

## Before using it

- This service uses a reverse-engineered web protocol, separate from Google's official API. Upstream changes can affect availability.
- Function calling uses prompt conventions. Some OpenAI parameters have no upstream equivalent; see [protocol coverage](api.md#protocol).
- Strict model checks are off by default. When enabled, a confirmed model mismatch becomes an error; a streaming client may already have received part of the response.
- Account access, context length and exit rate limits follow the web app. Verify the actual result of sensitive actions.
- Set an admin token and restrict access when deploying.

## Acknowledgments

- [zexadev/gemini-web2api-go](https://github.com/zexadev/gemini-web2api-go): the Go project this repository is based on.
- [bogdanfinn/tls-client](https://github.com/bogdanfinn/tls-client) — real Chrome TLS fingerprints
- [pkoukk/tiktoken-go](https://github.com/pkoukk/tiktoken-go) — BPE tokenizer
- [modernc.org/sqlite](https://gitlab.com/cznic/sqlite) — pure-Go SQLite (CGO-free, builds straight on alpine)

## License

MIT — see [LICENSE](../../LICENSE)

## Links

- [Sophomoresty/gemini-web2api](https://github.com/Sophomoresty/gemini-web2api) — a Python implementation of the same idea; its approach informed our early work on the Gemini web protocol
