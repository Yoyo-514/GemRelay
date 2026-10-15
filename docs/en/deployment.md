# Deployment and configuration

[Home](README.md) · [中文](../deployment.md)

Run commands from the repository root.

> Release and remote image URLs refer to the [upstream project](https://github.com/zexadev/gemini-web2api-go) and may not contain this version's additions. This repository's Compose file builds the current checkout by default instead of pulling upstream.

<a id="quick-start"></a>

## Quick start

### Prebuilt binary (simplest)

Grab the one for your platform from
[Releases](https://github.com/zexadev/gemini-web2api-go/releases). No Go, no Docker —
the single file is the whole thing:

```bash
chmod +x gemini-web2api-go_*
./gemini-web2api-go_* --port 8083 --admin-token your-admin-token
```

Data lands in `./data/gemrelay.db` by default; pass `--db /your/path.db` to move it.

To use MySQL / PostgreSQL, just set the `SQL_DSN` env var (unset = SQLite, nothing to change):

```bash
# MySQL (also accepts the native go-sql-driver form user:pass@tcp(host:3306)/dbname)
SQL_DSN="mysql://user:pass@host:3306/dbname"
# PostgreSQL
SQL_DSN="postgres://user:pass@host:5432/dbname?sslmode=disable"
```

Tables are created automatically; all three backends share one schema. Single instance? Stick with the default SQLite. You only need MySQL/PG when several instances share one pool.

### Docker (no source needed)

```bash
docker run -d --name gemini-web2api \
  -p 127.0.0.1:8083:8083 \
  -v "$PWD/data:/data" \
  -e ADMIN_TOKEN=your-admin-token \
  ghcr.io/zexadev/gemini-web2api-go:latest
```

With Docker Compose v2, run from the checkout's root. The configuration builds `gemrelay:local` and requires an admin token:

```bash
ADMIN_TOKEN=your-admin-token docker compose -f deployments/docker-compose.yml up -d --build
```

### From source

Use Go 1.27.2 or newer and run from the current checkout's root. Cloning upstream does not obtain this version's changes:

```bash
go build -o gemrelay ./cmd/gemrelay
./gemrelay --port 8083 --admin-token your-admin-token
```

The [deployment configuration](../../deployments/docker-compose.yml) already enables source builds and requires the full checkout; downloading that file alone is insufficient. The build context is the repository root and uses [build/package/Dockerfile](../../build/package/Dockerfile). The [.dockerignore](../../.dockerignore) allowlist includes production sources, module manifests and embedded assets only.

In PowerShell, set the environment variable before running Compose. Alternatively, use `docker compose --env-file .env -f deployments/docker-compose.yml up -d --build`; do not commit tokens or database credentials.

GemRelay stores container data in its own `gemrelay-data` volume. Existing project data stays in its original location; back it up before changing deployments. The current healthcheck only checks that the binary runs; use `GET /` for HTTP readiness.

Alternatively, build directly from the repository root: `docker build -f build/package/Dockerfile -t gemrelay .`.

Startup banner example from an older release (version and model list vary with the running build):

```
gemini-web2api-go v4.0.0
  Listening:   http://0.0.0.0:8083
  Base URL:    http://localhost:8083/v1
  API key:     sk-gemini-XX...XXXX  (mutable in admin UI)
  Admin UI:    http://localhost:8083/admin  (token auth)
  DB:          ./data/gemini.db
  Models:      [gemini-3.5-flash-lite gemini-3.6-flash]
  Cookie:      none (anonymous)
  Proxy:       none
  Impersonate: chrome_146
  Tokenizer:   tiktoken cl100k_base
  Per-IP 限流: 并发=5 / RPM=30 / RPH=80
  Retry:       3x / 2s
```

An API key is generated on first boot. The banner masks it; the full value is on the **Settings** page of the admin panel.

<a id="configuration"></a>

## Configuration

There are only two places to configure things, split by whether a change needs a restart.

### Runtime config → admin panel, Settings page

Saving publishes runtime settings and stores them in the database, taking precedence over `config.json` and flags. Existing network clients retain the timeout from creation; restart after changing the timeout to update all clients.

| Item | Notes |
|---|---|
| Default model | used when the client doesn't send `model` |
| Per-slot concurrency / RPM / RPH | rate limits, 0 = unlimited |
| Prompt byte cap | over the cap: with a cookie the history is sent as a text attachment (usable length up to ~160,000 bytes), without one the request is rejected with 400 `context_length_exceeded`. Neither path truncates silently. Counted in UTF-8 bytes (the upstream limit is byte-based, not token-based), default 128000. 0 = unlimited |
| Multi-turn (`multi_turn`) | off by default. When on, uses Gemini's native conversation_id server-side continuation — the client resends full history each turn, the server detects the continuation and sends only the newest message while history stays server-side, so long conversations no longer hit the single-request byte wall. Works signed-in or anonymous. **Note**: it does not enlarge the model's context window; content past the window is still evicted (recent-kept sliding window). It solves "long chat without hitting the wall + keep recent context", not "feed a huge document". |
| Auto-delete conversation (`auto_delete_conversation`) | Off by default. Attempts asynchronous deletion after successful signed-in single-turn generation; deletion failure does not change the response. The server-side multi-turn path does not run auto-delete |
| Temporary chat (`temporary_chat`) | off by default. When on, uses Gemini web's native **Temporary Chat** (`inner[45]=1`): the conversation never enters the account's chat list and isn't used for model training — cleaner than auto-delete, and it saves that extra delete request. Only meaningful signed-in; automatically inactive when server-side multi-turn is on (a temporary chat leaves no list entry, while multi-turn relies on the server keeping the conversation). |
| Anonymous-first (`anon_first`) | Off by default. With this enabled, a model that does not require login and a request without attachments may run anonymously. Tool definitions alone do not force login. Signed-in models, thinking tiers, media tools and attachments still need an account |
| Retry attempts / retry delay / upstream timeout | |
| Strict model check (`strict_model`) | Off by default. A confirmed mismatch between the requested model and the upstream-reported name returns `429 insufficient_quota` with `code: model_downgraded` before streaming starts. An open Chat stream sends the same error classification; Responses emits `response.failed`, without a successful terminal event. Real-time streaming is preserved: earlier content may already have reached the client and cannot be retracted. Missing model evidence, unknown models and media tools are allowed through. Aliases and thinking tiers share the base-model check; thinking-level downgrades are not detected. No automatic account retry is added. A mismatch may also mean the account lacks access, not necessarily exhausted quota. |
| Detail retention days | only request details expire; aggregates are kept forever |
| TLS fingerprint | `chrome_146` (default) / `chrome_144` / `chrome_133` / `firefox_147` / `safari_16_0` / `safari_ios_17_0` |
| Gemini `bl` version | upstream frontend build id, change it here when it expires |
| Proxy seed | `--proxy` imports a proxy at startup; manage it on the Proxy pool page afterwards. There is no separate runtime static-proxy fallback |
| Request logging | |

Every value is range-checked on the server (for example `retry_attempts` accepts 1-10, timeouts 5-600 seconds) and illegal values are rejected with a reason — browser-side limits are trivial to bypass, so the real gate is server-side.

### Credentials → also in the panel

| Item | Notes |
|---|---|
| API key | generated on first boot, rotatable or replaceable from the panel |
| Google cookie | Import on the Cookie pool page. An enabled account exposes signed-in models; actual access and quota still depend on upstream |

Both live in the database. The authenticated Settings page can display and copy the API key. Cookie lists return redacted summaries, not saved cookie contents.

### Deploy-time config → [deployment configuration](../../deployments/docker-compose.yml)

What's left is only what requires restarting the process:

| Item | Where |
|---|---|
| Listening port | `ports` + `command: --port` |
| Database path | `volumes` + `command: --db` |
| `ADMIN_TOKEN` | `environment`, the panel login token |

The `API_KEY` environment variable pins the API key (the panel can't change it), for deployments that must not be changed at runtime.

`--proxy` and `--cookie-file` are **seed parameters, not a second configuration layer**: at startup their values are imported into the proxy pool / cookie pool (deduplicated by URL and by cookie contents), and everything is managed from the panel afterwards. Change them and restart to take effect.

Command-line flags still work and are meant as temporary overrides during local debugging. Precedence: **panel changes > CLI flags / `config.json` > built-in defaults**.

When running the binary directly, copy [`configs/config.example.json`](../../configs/config.example.json) to `config.json` as a starting template. The program reads the explicit `--config` path, or `./config.json` in its working directory when the flag is omitted. Missing local configuration falls back to built-in defaults. Tunables in that file only seed the first boot; once changed in the panel, the panel wins.

All flags:

| Flag | Notes |
|---|---|
| `--port` | listening port, default 8083 |
| `--config` | path to `config.json` |
| `--db` | SQLite path, default `./data/gemrelay.db` |
| `--admin-token` | panel login token; empty = no auth (only acceptable when bound to 127.0.0.1) |
| `--api-key` | pins the `/v1/*` key so the panel can't change it, same as the `API_KEY` env var |
| `--cookie-file` | imports the cookie in this file into the cookie pool at startup |
| `--proxy` | imports this proxy into the proxy pool at startup |
| `--impersonate` | TLS fingerprint profile |
| `--version` | print version and exit (this is what the Docker healthcheck runs) |

---

Read more: [Deployment](deployment.md) · [API and models](api.md) · [Accounts and proxies](operations.md) · [Troubleshooting](troubleshooting.md) · [Development](development.md)
