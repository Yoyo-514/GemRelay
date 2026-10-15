# Admin panel, accounts and proxies

[Home](README.md) · [中文](../operations.md)

Run commands from the repository root.

<a id="admin"></a>

## Admin panel

`http://localhost:8083/admin`, log in with `--admin-token`.

- **Overview** — 24h KPIs, request volume / P50 latency dual-axis chart, per-model and per-proxy breakdowns, IP rate-limit usage, one-click connectivity check
- **Requests** — request log (metadata only, no prompt/response content), status/model filters, pagination
- **Proxy pool** — runtime CRUD, enable/disable, circuit breaker on repeated failures (each proxy is its own IP slot)
- **Cookie pool** — import multiple signed-in Google accounts; requests rotate through them **least-recently-used first**. One-click "check" tells you whether an account is still signed in, and **dead accounts are flagged in red** so you know which to re-import; `__Secure-1PSIDTS` is rotated automatically plus a 10-minute keepalive; each account is pinned to its own exit. The list shows redacted summaries only (cookie count / key entries / last 4 of SAPISID / failure count)
- **Settings** — runtime settings form (existing network clients need a restart to adopt timeout changes), API key rotation, read-only view of deploy-time config

### Usage quota

The Cookie pool's **Usage quota** column shows each account's remaining percentage and reset time
for the Gemini web app's **5-hour / weekly** limits. Opening the page queries up to three accounts
concurrently; there is no periodic polling. Refresh skips the server's 20-second cache. Disabled
accounts remain queryable. Missing buckets and failed requests are not shown as zero quota.

`GET /admin/api/quota[?account_id=N][&refresh=1]` uses the existing admin authentication.
Without an account ID it selects an enabled account. Query failures return HTTP 200 with
`ok:false` and `error`; an explicitly requested `account_id` is echoed even on failure.

```json
{"ok":true,"account":"my-account","account_id":1,"direct":false,
 "five_hour":{"used_percent":63,"remain_percent":37,"reset_at":1792050755,"reset_in_sec":13504},
 "weekly":{"used_percent":13,"remain_percent":87,"reset_at":1792486355,"reset_in_sec":503104},
 "fetched_at":1792037251}
```

The lookup does not advance account rotation, change health or exit bindings, or consume generation
rate-limit slots. It prefers the account's available proxy and respects `fallback_direct` when no
proxy is usable. Successful results are cached for 20 seconds by actual account, cookie and exit
identity; concurrent queries for the same identity share one upstream call. Failures are not cached.
The reverse-engineered web RPC `jSf9Qc` and parsing criteria follow the MIT project
[Nagi-ovo/voyager](https://github.com/Nagi-ovo/voyager) (`usageParsing.ts`). This independent endpoint
can stop working after upstream changes or for accounts without the usage page; generation is unaffected.

The frontend has an HTML shell, responsibility-specific [styles](../../web/admin/styles/) and
[scripts](../../web/admin/scripts/). All assets, including Chart.js, are embedded in the binary,
**not loaded from a CDN**, so no frontend build step or external asset connection is required.

**Serving it under a path prefix needs no extra configuration**: every URL in the panel is relative, so forwarding `https://example.com/gemini/` to this service's `/` is enough to open `https://example.com/gemini/admin`. (Requesting `/admin` without the trailing slash 301s to `admin/` — relative URLs resolve against the document URL's *directory*, and the two forms differ by one level, so they have to be normalised first.)

<a id="cookies"></a>

## Cookie (optional)

Attaching a Google account cookie makes requests run as a signed-in session. What it buys you is **`gemini-3.1-pro` plus its reasoning chain** (see [reasoning](api.md#reasoning)). Verified on a free account: six consecutive calls all reported `3.1 Pro`.

> Signed-in requests must carry an extra XSRF token. The project fetches it from the Gemini page automatically, caches it per cookie and re-fetches on expiry — nothing to configure. (Missing it makes **every** request fail with 400 while anonymous traffic keeps working — an earlier version hit exactly that.)

A signed-in session unlocks `gemini-3.1-pro` + reasoning chain, image/video input, **image generation (`gemini-image`) / music (`gemini-music`)** (see [models and media](api.md#models)), **canvas (`gemini-canvas`)** — an interactive HTML document returned inline — and **video generation (`gemini-video`, needs a Pro account)**. Deep research also needs a signed-in session but **is not implemented here yet**.

1. Sign in to [gemini.google.com](https://gemini.google.com)
2. DevTools (F12) → Application → Cookies → `https://gemini.google.com`
3. Copy: `SID` / `HSID` / `SSID` / `APISID` / `SAPISID` / `__Secure-1PSID` / `__Secure-1PSIDTS`
4. Add it in the panel under Cookie pool → Add account, or write it to `cookie.txt` and start with `--cookie-file cookie.txt` (imported into the pool at startup, managed from the panel afterwards):
```
SID=...; HSID=...; SSID=...; APISID=...; SAPISID=...; __Secure-1PSID=...; __Secure-1PSIDTS=...
```

The JSON form `{"cookie": "SID=...; ...", "sapisid": "..."}` is also accepted. Requests carrying `SAPISID` get a computed `SAPISIDHASH` authorization header, so that entry cannot be missing.

**The Cookie pool page is the only place cookies live.** Each request picks an enabled account least-recently-used first and advances the rotation. As soon as the pool holds an enabled account, `gemini-3.1-pro` appears in the model list.

An account's "failure count" and "last success" columns update automatically. Only **401/403 counts as the cookie's fault** — network errors, proxy failures and Google blocks (302) are excluded, because residential exits degrade often enough that counting them would turn the failure count into proxy noise and make healthy cookies look like the worst offenders.

Note that "last success" only means a request involving this cookie succeeded; it does **not** prove the cookie is still valid. An expired cookie doesn't error — Gemini just treats you as anonymous. Use the **Check** button in the list to tell "still signed in" from "expired/invalid" without spending a real conversation on it.

**The service attempts cookie renewal.** Success still depends on upstream session policy and device binding. Two things run together:

- Nearly every upstream response refreshes `SIDCC` / `__Secure-1PSIDCC` / `__Secure-3PSIDCC` via `Set-Cookie`, and we merge those back into the account.
- Every 10 minutes (first ping 15 seconds after boot) we hit `accounts.google.com/RotateCookies`: first the sentinel payload that mints a fresh `__Secure-1PSIDTS` (the ~30-minute ticket — without it Gemini treats you as anonymous), then the browser iframe flow that refreshes SIDCC. The same account is not pinged twice within 60 seconds, to avoid HTTP 429.

> **Export cookies from Firefox.** Recent Chrome enables Device Bound Session Credentials, so a session exported from Chrome is bound to that machine and `*PSIDTS` cannot be rotated — it dies in about 30 minutes to a few hours. Firefox has no such binding, and the same keepalive can attempt renewal, without a lifetime guarantee. If you must use Chrome: log in in a fresh private window, export immediately, and **close that window** so the browser does not rotate the same session out from under you.

**Each account is pinned to its own exit.** If the cookie pool and proxy pool rotated independently, one Google account would emit requests from dozens of different IPs, which is exactly what account sharing looks like to Google. An account binds to the first exit it uses and stays there until that exit becomes unusable.

**A dead account no longer kills the request** — the next account in the pool is tried instead. Otherwise a bigger pool would only mean more ways to hit a bad one.

<a id="proxies"></a>

## Proxy pool (the core of running this for free)

The measurements in this section are historical results recorded by the upstream project, not capacity or recovery-time guarantees. Different proxy URLs sharing one public IP still share upstream IP limits.


**Why you need it**: a single IP sending in bursts eventually gets redirected to `google.com/sorry/index`. That threshold is **80-180 requests**, and the wide range is driven by **connection strategy, exit quality and pacing** together:

| Exit | Connection strategy | Concurrency | Pacing | Successful requests when blocked |
|---|---|---|---|---|
| residential | Reused connection pool | 10 | no delay | 151 / 172 / 177 |
| residential | Reused connection pool | 3 | no delay | 103 / 111 |
| residential | Fresh connection each time | 10 | no delay | 106 / 109 |
| residential | Fresh connection each time | 1 | 24s apart | 81 / 166 |
| static | Reused connection pool | 10 | no delay | 188 |
| static | Reused connection pool | 1 | **10 per minute** | **800, never blocked** |

Only a 302 to `/sorry/` counts as blocked, and every exit was pre-screened.

**Connection reuse is worth about 60%**: concurrency pinned at 10, both arms started together, each run lasting only 80 seconds (too short for exits to drift) — reused connections reached 172/177, fresh connections 106/109.

**A steady pace matters more than anything else.** On the same static IP, a burst run was blocked after 188 requests, while a steady 10 requests/minute ran **800 requests over 110 minutes without ever being blocked**. The default `per_ip_rph=80` is therefore a very conservative floor; a deployment that deliberately paces itself can raise it a lot.

> This section previously said "slowing down does not help", based on the burst arm (103-177) and the slow arm (81-166) overlapping almost completely on residential exits. The observation was right but the attribution was wrong: residential exits degrade over long runs (6 of 8 pre-screened exits exceeded a 40% failure rate partway through the slow run), and that degradation swamped the effect of pacing. Once a static IP removes that confound, pacing turns out to matter a great deal.

**Proxy failures consume the budget early**: the dirtier the path, the sooner the block, because some of those "failed" requests did reach Google and were counted (the slow arm actually sent 195 requests to get 166 successes, 17% more than the success count suggests). Don't expect to squeeze out capacity by retrying failures.

**Once blocked, the block is hard, and it clears after roughly two hours.** Two independent re-probes of 30 requests each, 20s apart, spanning about 10 minutes: **60 requests, zero successes**. Continued probing recovered somewhere between **106 and 121 minutes**. That is where the proxy pool's default 120-minute cooldown comes from.

Control group: 10 IPs × 50 requests each (418 requests) produced zero rejections from Google. The default `per_ip_rph=80` sits at the low end of the measured burst range.

**The fix**: add several proxies on the Proxy pool page. **Each proxy entry is an independent local slot** with its own concurrency/RPM/RPH allowance, so N proxies means N times the total capacity.

Supported proxy schemes:
- `http://user:pass@host:port`
- `https://user:pass@host:port`
- `socks5://user:pass@host:port`
- `socks5h://user:pass@host:port` (remote DNS resolution, avoids local DNS poisoning)

**Scheduling rules**:
- Once any proxy is configured, requests **do not fall back to a direct connection by default**; `fallback_direct` explicitly allows that fallback (so a full pool can't end up burning the host IP)
- 5 failures trip the circuit breaker (resettable from the panel)
- All proxies full → HTTP 429 (no Google quota consumed; retry once a slot frees up)

**Environment variables `HTTPS_PROXY` / `ALL_PROXY` are ignored.** Proxies come only from the proxy pool (`--proxy` / `config.json` merely seed the pool at startup). Otherwise a stray `export` on the host would silently change the exit IP while the panel still showed a direct connection, which makes troubleshooting misleading.

## Fingerprint simulation

Direct connections use [bogdanfinn/tls-client](https://github.com/bogdanfinn/tls-client) with the browser profile selected by `impersonate`, Chrome 146 by default. This does not guarantee browser verification or exemption from upstream risk controls.

Proxied connections use Go's standard `net/http` client with `http.ProxyURL`. Application headers (`Sec-CH-UA`, `Sec-Fetch-*`, `User-Agent`) still follow the selected browser profile, but that does not give the connection the browser's TLS fingerprint.

**Note that through a proxy the TLS fingerprint is still ours, not the exit node's.** HTTP CONNECT only builds a tunnel; the TLS handshake is end-to-end between us and Google. JA3 echo tests confirm it: through the same proxy, stdlib and tls-client produce different JA3s (so the proxy isn't rewriting anything), while stdlib direct and stdlib through a proxy produce identical JA3s (so the proxy layer doesn't touch TLS). **So once a proxy is configured, what Google sees is the Go standard library's fingerprint, not Chrome 146's.**

This does not change the blocking threshold, though: in a same-start comparison, tls-client Chrome_146 reached 111 requests and Go stdlib 103 — a gap of 8, while the variance between exits under identical settings reaches 36% (151 vs 111). Making the proxy path use the Chrome fingerprint needs a different justification (long-term account profiling, say); it won't buy throughput.

**An observation from testing**: a naive SDK call (e.g. Python urllib) that trips risk control gets **HTTP 429**; once disguised as Chrome, tripping it yields **HTTP 302** to `google.com/sorry/index` (the CAPTCHA page). Both indicate upstream risk controls; a redirect alone does not prove that the client passed browser verification.

<a id="privacy"></a>

## Privacy

- **Prompt and response bodies are never stored** — only metadata: model, proxy id, latency, token counts, status code, error type
- Legacy `prompt_preview` / `response_preview` columns are dropped automatically when migrating from older versions
- Token counts use [tiktoken-go](https://github.com/pkoukk/tiktoken-go) cl100k_base when initialized. If loading fails, the implementation falls back to UTF-8 byte length divided by four. Both are local estimates of Gemini usage
- Gemini's web app does not return real token counts (no token/usage field appears anywhere in the response), so local estimation is the only option

---

Read more: [Deployment](deployment.md) · [API and models](api.md) · [Accounts and proxies](operations.md) · [Troubleshooting](troubleshooting.md) · [Development](development.md)
