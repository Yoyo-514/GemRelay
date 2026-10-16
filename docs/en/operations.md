# Admin panel, accounts and proxies

[Home](README.md) · [中文](../operations.md)

Run commands from the repository root.

<a id="admin"></a>

## Admin panel

`http://localhost:8083/admin`, log in with `--admin-token`.

- **Overview** — 24h KPIs, request volume / P50 latency dual-axis chart, per-model and per-proxy breakdowns, IP rate-limit usage, one-click connectivity check
- **Requests** — metadata records, status/model filters and pagination
- **Proxy pool** — runtime CRUD, enable/disable, circuit breaker on repeated failures (each proxy is its own IP slot)
- **Cookie pool** — import multiple signed-in Google accounts; requests rotate through them **least-recently-used first**. One-click "check" tells you whether an account is still signed in, and **dead accounts are flagged in red** so you know which to re-import; `__Secure-1PSIDTS` is rotated automatically plus a 10-minute keepalive; each account is pinned to its own exit. The list shows redacted summaries only (cookie count / key entries / last 4 of SAPISID / failure count)
- **Settings** — runtime settings form (existing network clients need a restart to adopt timeout changes), API key rotation, read-only view of deploy-time config

### Usage quota

The Cookie pool's **Usage quota** column shows each account's remaining percentage and reset time
for the Gemini web app's **5-hour / weekly** limits. While visible, the page queries up to three
accounts concurrently and waits 60 seconds after each round. Adjust `quota_poll_interval_sec`
in Settings (10–3600 seconds). Leaving the page, hiding the tab or signing out pauses polling;
returning to the visible page resumes it.
Successful queries show quota and reset times. A failed refresh keeps the last successful values
with an amber cached-data label and the error. Recovery updates the values and clears the label.
An initial failure shows the error. The Refresh button skips the server's 20-second cache.
Disabled accounts remain queryable; missing buckets show “not provided”.

`GET /admin/api/quota[?account_id=N][&refresh=1]` uses the existing admin authentication.
Without an account ID it selects an enabled account. Query failures return HTTP 200 with
`ok:false` and `error`; an explicitly requested `account_id` is echoed even on failure.

```json
{"ok":true,"account":"my-account","account_id":1,"direct":false,
 "five_hour":{"used_percent":63,"remain_percent":37,"reset_at":1792050755,"reset_in_sec":13504},
 "weekly":{"used_percent":13,"remain_percent":87,"reset_at":1792486355,"reset_in_sec":503104},
 "fetched_at":1792037251}
```

The lookup uses a separate read-only path, leaving account rotation, health, exit bindings and
generation rate-limit counts unchanged. It prefers the account's available proxy; `fallback_direct`
controls direct fallback. Successful results are stored by actual account, cookie and exit identity;
admin queries reuse them for 20 seconds. Concurrent queries for the same identity share one upstream
call. A failure clears the server snapshot; the page retains its last successful result for the current visit.

### Quota-aware account selection

Enable `quota_aware_pick` in Settings to rank accounts by the lower remaining value across the two
quota windows. The priority is sufficient quota (≥10%), unknown quota, then low quota (<10%).
Sufficient accounts are grouped into 20-percentage-point bands. Within a band, health, least-recently-used
time and account ID determine the order. When every account has low quota, selection uses the highest
remaining value.

Selection shares the admin query's 20-second freshness window. Missing or expired snapshots,
including those reaching a quota reset time, count as unknown and trigger an asynchronous lookup.
Subsequent requests use the updated values. Lookups run up to three at a time, with at most one
start per identity every 20 seconds. Single-turn requests, account retries and new
multi-turn sessions share this policy. Continuing sessions retain their account. The default setting
is off and uses the existing account order.

This policy distributes requests; account access and actual quota remain upstream decisions.
Strict model checks can be enabled separately to detect model changes in responses.

The reverse-engineered web RPC `jSf9Qc` and parsing criteria follow the MIT project
[Nagi-ovo/voyager](https://github.com/Nagi-ovo/voyager) (`usageParsing.ts`). This independent endpoint
can fail after upstream changes or for accounts lacking the usage page, independently of generation.

The frontend has an HTML shell, responsibility-specific [styles](../../web/admin/styles/) and
[scripts](../../web/admin/scripts/). All assets, including Chart.js, are embedded in the binary
for local loading in intranet or offline deployments, using native scripts and styles.

The panel uses relative URLs and supports reverse-proxy path prefixes. Forward `https://example.com/gemini/` to this service's `/` to access `https://example.com/gemini/admin`. Requests to `/admin` receive a 301 redirect to `admin/`, giving relative resources a consistent base directory.

<a id="cookies"></a>

## Cookie (optional)

Google account cookies enable signed-in requests for `gemini-3.1-pro` and [reasoning](api.md#reasoning), image/video input, [image and music generation](api.md#models), canvas and video generation (Pro account required). Tests recorded six consecutive free-account calls reporting `3.1 Pro`. Deep research's multi-step async flow is currently unimplemented.

Signed-in requests require an XSRF token. The service fetches it from the Gemini page, caches it per
cookie and refreshes it on expiry. A missing token causes signed-in requests to return 400;
anonymous requests continue normally.

1. Sign in to [gemini.google.com](https://gemini.google.com)
2. DevTools (F12) → Application → Cookies → `https://gemini.google.com`
3. Copy: `SID` / `HSID` / `SSID` / `APISID` / `SAPISID` / `__Secure-1PSID` / `__Secure-1PSIDTS`
4. Add it in the panel under Cookie pool → Add account, or write it to `cookie.txt` and start with `--cookie-file cookie.txt` (imported into the pool at startup, managed from the panel afterwards):
```
SID=...; HSID=...; SSID=...; APISID=...; SAPISID=...; __Secure-1PSID=...; __Secure-1PSIDTS=...
```

The JSON form `{"cookie": "SID=...; ...", "sapisid": "..."}` is also accepted. `SAPISID` is required to compute the `SAPISIDHASH` authorization header.

The Cookie pool manages account cookies. Each request picks an enabled account least-recently-used first and advances the rotation. An enabled account makes `gemini-3.1-pro` appear in the model list.

An account's failure count and last-success time update with requests. Only 401/403 counts as a credential failure. Network errors, proxy failures and Google 302 blocks are classified separately to distinguish credential problems from exit quality.

Last success records a successful request using the cookie. Upstream may treat an expired cookie as anonymous, allowing text requests to succeed. Use the Check button to distinguish a valid signed-in session from an expired or invalid one.

**The service attempts cookie renewal.** Success still depends on upstream session policy and device binding. Two things run together:

- Nearly every upstream response refreshes `SIDCC` / `__Secure-1PSIDCC` / `__Secure-3PSIDCC` via `Set-Cookie`; the service merges those values into the account.
- Every 10 minutes, starting 15 seconds after boot, the service calls `accounts.google.com/RotateCookies`: a sentinel payload renews `__Secure-1PSIDTS`, then the browser iframe flow refreshes SIDCC. The ticket lasts about 30 minutes; expiry causes anonymous treatment. Attempts for one account are spaced at least 60 seconds apart to avoid HTTP 429.

> Prefer Firefox for cookie export. Its sessions support renewal attempts through the service's keepalive; actual lifetime depends on upstream policy.
> Recent Chrome uses Device Bound Session Credentials. Exported sessions may expire within 30 minutes to a few hours and fail `*PSIDTS` renewal.
> With Chrome, sign in through a fresh private window, export immediately and close the window so the service handles subsequent session rotation.

An account binds to its first exit and changes it when that exit becomes unusable. This avoids frequent cross-IP requests from independent account/proxy rotation, which Google may identify as account sharing.

When an account fails authentication, the service tries the next account in the pool.

<a id="proxies"></a>

## Proxy pool

These measurements are historical test records. Actual capacity and recovery time vary with the exit and upstream policy. Proxy URLs sharing a public IP also share its upstream quota.


Historical tests saw a single IP redirected to `google.com/sorry/index` after about **80–180 burst requests**, depending on connection strategy, exit quality and pacing:

| Exit | Connection strategy | Concurrency | Pacing | Successful requests when blocked |
|---|---|---|---|---|
| residential | Reused connection pool | 10 | no delay | 151 / 172 / 177 |
| residential | Reused connection pool | 3 | no delay | 103 / 111 |
| residential | Fresh connection each time | 10 | no delay | 106 / 109 |
| residential | Fresh connection each time | 1 | 24s apart | 81 / 166 |
| static | Reused connection pool | 10 | no delay | 188 |
| static | Reused connection pool | 1 | **10 per minute** | **800, never blocked** |

Only a 302 to `/sorry/` counts as blocked, and every exit was pre-screened.

The connection-reuse group completed about 60% more requests. Both groups used concurrency 10, started together and ran for 80 seconds to reduce exit drift: reused connections reached 172/177 requests, fresh connections 106/109.

Pacing had a clear effect on the static IP: a burst run was blocked after 188 requests, while 10 requests/minute completed 800 requests over 110 minutes with all passing. The default `per_ip_rph=80` takes the lower end of the burst range; paced deployments can adjust it based on exit behavior.

> This section previously said "slowing down does not help", based on overlapping burst (103-177) and slow (81-166) results on residential exits. Later tests identified exit degradation as a confound: 6 of 8 pre-screened exits exceeded a 40% failure rate partway through the slow run. Static-IP tests made the pacing effect clearer.

Some failed requests reach Google and consume quota. The slow group sent 195 requests for 166 successes, 17% more than the success count; include retries when planning exit capacity.

After blocking, two independent runs of 30 probes, 20 seconds apart over about 10 minutes each, all failed. Continued probing recovered between **106 and 121 minutes**. This record informed the default 120-minute proxy cooldown.

Another control group recorded 10 IPs × 50 requests each (418 requests), with zero Google rejections.

Add proxies on the Proxy pool page. Each entry provides an independent local IP slot with its own concurrency/RPM/RPH limits. N entries provide N sets of local limits; upstream capacity also depends on their actual public exits.

Supported proxy schemes:
- `http://user:pass@host:port`
- `https://user:pass@host:port`
- `socks5://user:pass@host:port`
- `socks5h://user:pass@host:port` (remote DNS resolution, avoids local DNS poisoning)

**Scheduling rules**:
- Configured proxies are used exclusively by default; enable `fallback_direct` to allow direct fallback
- 5 failures trip the circuit breaker (resettable from the panel)
- Full proxy pool → local HTTP 429; retry after a slot frees up

The proxy pool manages exits, seeded at startup through `--proxy` / `config.json`. The service ignores `HTTPS_PROXY` / `ALL_PROXY`, keeping actual routing consistent with the panel.

## Fingerprint simulation

Direct connections use [bogdanfinn/tls-client](https://github.com/bogdanfinn/tls-client) with the browser profile selected by `impersonate`, Chrome 146 by default. Browser fingerprint simulation remains subject to Google risk controls and exit limits.

Proxied connections use Go's standard `net/http` client with `http.ProxyURL`, so their TLS fingerprint comes from the Go client. Application headers (`Sec-CH-UA`, `Sec-Fetch-*`, `User-Agent`) follow the selected browser profile. HTTP CONNECT establishes a tunnel; TLS runs end-to-end between the client and Google. JA3 tests through the same proxy produced different results for stdlib and tls-client, while stdlib direct and proxied results matched.

In a same-start comparison, tls-client Chrome_146 reached 111 requests and Go stdlib 103, a gap of 8. Differences between exits under identical settings reached 36% (151 vs 111). Exit quality had the larger effect in these tests; adopting a Chrome fingerprint for proxies would need to consider other concerns, such as long-term account profiling.

Tests also recorded HTTP 429 for ordinary SDK requests such as Python urllib, and HTTP 302 to `google.com/sorry/index` (the CAPTCHA page) for Chrome-simulated requests. Both indicate upstream risk controls.

<a id="privacy"></a>

## Privacy

- Database request records contain metadata only: model, proxy ID, latency, token counts, status code and error type
- Legacy `prompt_preview` / `response_preview` columns are dropped automatically when migrating from older versions
- Token counts use [tiktoken-go](https://github.com/pkoukk/tiktoken-go) cl100k_base when initialized. If loading fails, the implementation falls back to UTF-8 byte length divided by four. Both are local estimates of Gemini usage
- Observed Gemini web responses lack token/usage fields, so usage is estimated locally

---

Read more: [Deployment](deployment.md) · [API and models](api.md) · [Accounts and proxies](operations.md) · [Troubleshooting](troubleshooting.md) · [Development](development.md)
