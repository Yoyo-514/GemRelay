# Deployment and configuration

[Home](README.md) · [中文](../deployment.md)

Deploy with the published Docker image or a binary. The image examples use the `latest` tag; replace `your-admin-token` with your own admin token.

<a id="quick-start"></a>

## Quick start

### Docker image

The image is available on [GHCR](https://github.com/Yoyo-514/GemRelay/pkgs/container/gemrelay) for Linux amd64 / arm64. Run on a Linux deployment machine:

```bash
docker pull ghcr.io/yoyo-514/gemrelay:latest
docker run -d --name gemrelay \
  -p 0.0.0.0:8083:8083 \
  -v "gemrelay-data:/data" \
  ghcr.io/yoyo-514/gemrelay:latest \
  --db "/data/gemrelay.db" --port 8083 --admin-token your-admin-token
```

The database is stored at `/data/gemrelay.db` in the container and persisted in the `gemrelay-data` volume. Back up the data before upgrading. The port binds to `0.0.0.0` on the deployment machine for access through its public or LAN IP.

Version tags (`vX.Y.Z`) identify releases. The `main` and `latest` tags track main-branch builds. To pin a release, use its tag from the release page.

### Docker Compose

Docker Compose v2 works on Linux and Windows Docker Desktop. Save this as `compose.yaml` in your deployment directory:

```yaml
services:
  gemrelay:
    image: ghcr.io/yoyo-514/gemrelay:latest
    container_name: gemrelay
    restart: unless-stopped
    ports:
      - "0.0.0.0:8083:8083"
    volumes:
      - gemrelay-data:/data
    command: ["--db", "/data/gemrelay.db", "--port", "8083", "--admin-token", "your-admin-token"]

volumes:
  gemrelay-data:
```

Run from that directory:

```bash
docker compose -f "compose.yaml" pull
docker compose -f "compose.yaml" up -d
```

### Binary

Download the executable for your system and architecture from the [latest release](https://github.com/Yoyo-514/GemRelay/releases/latest). Release filenames include a version number; choose by the suffix below:

| System | amd64 (x86-64) | arm64 |
|---|---|---|
| Linux | `*_linux_amd64` | `*_linux_arm64` |
| macOS | `*_darwin_amd64` | `*_darwin_arm64` |
| Windows | `*_windows_amd64.exe` | `*_windows_arm64.exe` |

Rename the downloaded program to `gemrelay` on Linux / macOS, or `gemrelay.exe` on Windows.

On Linux / macOS, grant execute permission and start it from the program's directory:

```bash
chmod +x "./gemrelay"
"./gemrelay" --port 8083 --admin-token your-admin-token
```

On Windows, run from the program's directory in PowerShell:

```powershell
& "./gemrelay.exe" --port 8083 --admin-token your-admin-token
```

The binary stores data in `./data/gemrelay.db` relative to its working directory. Use `--db` to choose another path.

### Sign in and connect

The service listens on `0.0.0.0:8083` by default. Replace `SERVER_IP` with the server's public or LAN IP, and allow TCP port 8083 in the host firewall and cloud security group.

Open `http://SERVER_IP:8083/admin` and sign in with the admin token. The service generates an API key on first boot. Startup output shows a masked value; the full key is on the Settings page.

Set the client's base URL to `http://SERVER_IP:8083/v1` and use the key from the panel. Request examples are in [API, models and clients](api.md).

## Database

SQLite is the default database. To use MySQL / PostgreSQL, set `SQL_DSN`:

```bash
# MySQL (also accepts the native go-sql-driver form user:pass@tcp(host:3306)/dbname)
SQL_DSN="mysql://user:pass@host:3306/dbname"
# PostgreSQL
SQL_DSN="postgres://user:pass@host:5432/dbname?sslmode=disable"
```

Tables are created automatically; all three backends share one schema. SQLite suits a single instance; MySQL / PostgreSQL can serve multiple instances sharing pool data.

## Build from source

### Go binary

Use Go 1.27.2 or newer and run from the repository root:

```bash
go build -o gemrelay ./cmd/gemrelay
./gemrelay --port 8083 --admin-token your-admin-token
```

### Local Docker image

Build and run from the repository root:

```bash
docker build -f build/package/Dockerfile -t gemrelay:local .
docker run -d --name gemrelay \
  -p 0.0.0.0:8083:8083 \
  -v gemrelay-data:/data \
  -e ADMIN_TOKEN=your-admin-token \
  gemrelay:local
```

The repository's [Compose configuration](../../deployments/docker-compose.yml) pulls the published image by default. For a local build, set `image` to `gemrelay:local`, set `pull_policy` to `build`, and enable the commented `build` block. The repository root is the build context, using [build/package/Dockerfile](../../build/package/Dockerfile). The [.dockerignore](../../.dockerignore) allowlist includes production sources, module manifests and embedded assets.

After enabling the local build, provide an admin token and start the service in Git Bash or a Linux terminal:

```bash
ADMIN_TOKEN=your-admin-token docker compose -f deployments/docker-compose.yml up -d --build
```

In PowerShell, set `ADMIN_TOKEN` before running Compose's `up -d --build`. Alternatively, use `docker compose --env-file ".env" -f "deployments/docker-compose.yml" up -d --build` to read a local environment file. Keep files containing tokens or database credentials local.

The repository Compose healthcheck checks that the binary runs. Use `GET /` for HTTP readiness.

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

### Startup configuration

These settings take effect after restarting the process. For Docker deployments, also configure port mappings and volumes in Compose or `docker run`:

| Item | Where |
|---|---|
| Listening address | `host` in `config.json`, default `0.0.0.0` |
| Listening port | `--port`; map the container port with `ports` or `-p` |
| Database path | `--db`; persist container data with `volumes` or `-v` |
| Admin token | `--admin-token` or `ADMIN_TOKEN`, used to sign in to the panel |

The `API_KEY` environment variable fixes the key at deployment time and makes it read-only in the panel.

`--proxy` and `--cookie-file` import values into the proxy and cookie pools at startup, deduplicated by URL and cookie contents respectively. Manage the pools from the panel afterwards. Restart after changing the flags to import new values.

Startup flags and `config.json` specify deployment settings and initial runtime values. Precedence: **panel changes > CLI flags / `config.json` > built-in defaults**.

When running the binary directly, copy [`configs/config.example.json`](../../configs/config.example.json) to `config.json` as a starting template. The program reads the explicit `--config` path, or `./config.json` in its working directory when the flag is omitted. Missing local configuration falls back to built-in defaults. Tunables in that file only seed the first boot; once changed in the panel, the panel wins.

All flags:

| Flag | Notes |
|---|---|
| `--port` | listening port, default 8083 |
| `--config` | path to `config.json` |
| `--db` | SQLite path, default `./data/gemrelay.db` |
| `--admin-token` | Panel login token; set it for public or LAN deployments. Local debugging bound to a loopback address can leave it empty to disable authentication |
| `--api-key` | Fixes the `/v1/*` key at deployment time and makes it read-only in the panel, equivalent to the `API_KEY` environment variable |
| `--cookie-file` | imports the cookie in this file into the cookie pool at startup |
| `--proxy` | imports this proxy into the proxy pool at startup |
| `--impersonate` | TLS fingerprint profile |
| `--version` | print version and exit (this is what the Docker healthcheck runs) |

---

Read more: [Deployment](deployment.md) · [API and models](api.md) · [Accounts and proxies](operations.md) · [Troubleshooting](troubleshooting.md) · [Development](development.md)
