# Deployment and configuration

[Home](README.md) · [中文](../deployment.md)

Run commands from the repository root.

<a id="quick-start"></a>

## Quick start

### Prebuilt binary

Download the single-file executable for your platform from
[Releases](https://github.com/Yoyo-514/GemRelay/releases) and run it directly:

```bash
chmod +x gemrelay_*
./gemrelay_* --port 8083 --admin-token your-admin-token
```

Data lands in `./data/gemrelay.db` by default; pass `--db /your/path.db` to move it.

SQLite is the default database. To use MySQL / PostgreSQL, set `SQL_DSN`:

```bash
# MySQL (also accepts the native go-sql-driver form user:pass@tcp(host:3306)/dbname)
SQL_DSN="mysql://user:pass@host:3306/dbname"
# PostgreSQL
SQL_DSN="postgres://user:pass@host:5432/dbname?sslmode=disable"
```

Tables are created automatically; all three backends share one schema. SQLite suits a single instance; MySQL / PostgreSQL can serve multiple instances sharing pool data.

### Local Docker image

Build and run from the repository root:

```bash
docker build -f build/package/Dockerfile -t gemrelay:local .
docker run -d --name gemrelay \
  -p 127.0.0.1:8083:8083 \
  -v gemrelay-data:/data \
  -e ADMIN_TOKEN=your-admin-token \
  gemrelay:local
```

With Docker Compose v2, run from the checkout's root. The configuration builds `gemrelay:local` and requires an admin token:

```bash
ADMIN_TOKEN=your-admin-token docker compose -f deployments/docker-compose.yml up -d --build
```

### From source

Use Go 1.27.2 or newer and run from this checkout's root:

```bash
go build -o gemrelay ./cmd/gemrelay
./gemrelay --port 8083 --admin-token your-admin-token
```

The [deployment configuration](../../deployments/docker-compose.yml) builds from the full checkout. The repository root is the build context, using [build/package/Dockerfile](../../build/package/Dockerfile). The [.dockerignore](../../.dockerignore) allowlist includes production sources, module manifests and embedded assets.

In PowerShell, set the environment variable before running Compose. Alternatively, use `docker compose --env-file .env -f deployments/docker-compose.yml up -d --build`; keep files containing tokens or database credentials local.

GemRelay stores container data in its own `gemrelay-data` volume; back up your data before changing deployments. The current healthcheck only checks that the binary runs; use `GET /` for HTTP readiness.

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

Settings are grouped into runtime and deploy-time configuration. Credentials are managed in the admin panel.

### Runtime config → admin panel, Settings page

Saving publishes runtime settings and stores them in the database, taking precedence over `config.json` and flags. Existing network clients retain the timeout from creation; restart after changing the timeout to update all clients.

| Item | Notes |
|---|---|
| Default model | used when the client doesn't send `model` |
| Per-slot concurrency / RPM / RPH | rate limits, 0 = unlimited |
| Prompt byte cap | Measured in UTF-8 bytes, default 128000; 0 disables the local check. Over the cap, signed-in requests send the history as a text attachment (usable length ~160,000 bytes), while anonymous requests return 400 `context_length_exceeded`. The original text is preserved |
| Multi-turn (`multi_turn`) | Off by default. Uses Gemini's native conversation_id: the client sends full history each turn, and a matching continuation forwards only the newest message while upstream retains history. Works signed-in or anonymously and reduces request-body size. The model's context window stays the same; early content is evicted when it overflows, and large documents remain subject to input limits |
| Auto-delete conversation (`auto_delete_conversation`) | Off by default. Applies to signed-in single-turn requests and attempts asynchronous deletion after successful generation. A deletion failure leaves the generated response intact |
| Temporary chat (`temporary_chat`) | Off by default. Uses native Temporary Chat (`inner[45]=1`), excluding the conversation from the account's chat list and model training and saving the later deletion request. Applies to signed-in single-turn requests; multi-turn continuation uses persistent conversations |
| Anonymous-first (`anon_first`) | Off by default. Prefers anonymous execution for models that allow it when the request contains text only, including requests with tool definitions. Signed-in models, thinking tiers, media tools and attachments use an account |
| Quota-aware selection (`quota_aware_pick`) | Off by default. Ranks accounts by quota band, preserving health and rotation order within a band. Reads snapshots up to 20 seconds old and refreshes them asynchronously on demand. See [quota-aware selection](operations.md#quota-aware-account-selection) |
| Quota polling interval (`quota_poll_interval_sec`) | Default 60 seconds, range 10–3600. The visible Cookie pool page refreshes in rounds with up to three concurrent queries |
| Retry attempts / retry delay / upstream timeout | |
| Strict model check (`strict_model`) | Off by default. A confirmed mismatch ends the request: before streaming starts, it returns `429 insufficient_quota` with `code: model_downgraded`; an open Chat stream sends the same error classification, while Responses emits `response.failed`. Both end in failure. Output continues to stream in real time, so the client may already have received some text. Checks cover known text models with model evidence; aliases and thinking tiers use the base-model criteria, with thinking depth outside the check's scope. Other cases pass through. Account retries are left to the caller. A mismatch may indicate exhausted quota or missing account access |
| Detail retention days | only request details expire; aggregates are kept forever |
| TLS fingerprint | `chrome_146` (default) / `chrome_144` / `chrome_133` / `firefox_147` / `safari_16_0` / `safari_ios_17_0` |
| Gemini `bl` version | upstream frontend build id, change it here when it expires |
| Request logging | |

The server validates every configuration value, for example `retry_attempts` from 1–10 and timeouts from 5–600 seconds. Values outside the range are rejected with a reason.

### Credentials → also in the panel

| Item | Notes |
|---|---|
| API key | generated on first boot, rotatable or replaceable from the panel |
| Google cookie | Import on the Cookie pool page. An enabled account exposes signed-in models; actual access and quota still depend on upstream |

Both are stored in the database. The authenticated Settings page can display and copy the API key. Cookie lists return redacted summaries only.

### Deploy-time config → [deployment configuration](../../deployments/docker-compose.yml)

These settings take effect after restarting the process:

| Item | Where |
|---|---|
| Listening port | `ports` + `command: --port` |
| Database path | `volumes` + `command: --db` |
| `ADMIN_TOKEN` | `environment`, the panel login token |

The `API_KEY` environment variable fixes the key at deployment time and makes it read-only in the panel.

`--proxy` and `--cookie-file` import values into the proxy and cookie pools at startup, deduplicated by URL and cookie contents respectively. Manage the pools from the panel afterwards. Restart after changing the flags to import new values.

Command-line flags still work and are meant as temporary overrides during local debugging. Precedence: **panel changes > CLI flags / `config.json` > built-in defaults**.

When running the binary directly, copy [`configs/config.example.json`](../../configs/config.example.json) to `config.json` as a starting template. The program reads the explicit `--config` path, or `./config.json` in its working directory when the flag is omitted. Missing local configuration falls back to built-in defaults. Tunables in that file only seed the first boot; once changed in the panel, the panel wins.

All flags:

| Flag | Notes |
|---|---|
| `--port` | listening port, default 8083 |
| `--config` | path to `config.json` |
| `--db` | SQLite path, default `./data/gemrelay.db` |
| `--admin-token` | Panel login token; leaving it empty disables authentication and is suitable only for local deployments bound to 127.0.0.1 |
| `--api-key` | Fixes the `/v1/*` key at deployment time and makes it read-only in the panel, equivalent to the `API_KEY` environment variable |
| `--cookie-file` | imports the cookie in this file into the cookie pool at startup |
| `--proxy` | imports this proxy into the proxy pool at startup |
| `--impersonate` | TLS fingerprint profile |
| `--version` | print version and exit (this is what the Docker healthcheck runs) |

---

Read more: [Deployment](deployment.md) · [API and models](api.md) · [Accounts and proxies](operations.md) · [Troubleshooting](troubleshooting.md) · [Development](development.md)
