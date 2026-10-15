# Limitations and troubleshooting

[Home](README.md) · [中文](../troubleshooting.md)

Run commands from the repository root.

<a id="limitations"></a>

## Limitations

- **Per-IP ceiling**: when sending in bursts, measured at **80-180 requests** before the sorry-page redirect (connection reuse buys about 60%: at concurrency 10, a reused pool reached 172/177 versus 106/109 for a fresh connection per request). But **a steady pace barely reaches the ceiling at all** — 10 requests/minute on a static IP ran 800 requests without a block. `per_ip_rph=80` sits at the bottom of the burst range → use the proxy pool to scale, or pace yourself and raise the limit
- **Signed-in features**: image generation (`gemini-image`), music (`gemini-music`), canvas (`gemini-canvas`) and video generation (`gemini-video`, Pro accounts only — free accounts are refused by the content policy) are implemented; deep research is not (a multi-step async flow)
- **Function calling**: prompt-level, the model doesn't always answer in the expected format (a real protocol layer isn't available to us)
- **Multimodal**: image/video input needs a cookie. Image, music, canvas and video generation work with a cookie; video generation additionally needs a Pro/paid account
- **Long context hits two walls**: ~130,000 bytes for the request body and ~160,000 bytes for attachments (the latter is the **total** amount of content the model can see — splitting it across several attachments does not raise the budget). A cookie only takes the usable length from 130K to ~160K; genuinely long conversations still have to be compacted by the client
- **Token counts**: tiktoken estimates (Gemini's real tokenizer is not public), within about ±20% of the true value
- **Cookie pool never auto-removes a bad account**: outcomes are written back (only 401/403 count as the cookie's fault — network errors and 302 blocks don't), but failures never trigger an automatic disable, so you have to do it from the panel. Also `last_ok_at` only means "a request using this cookie succeeded", not that the cookie is still valid — an expired cookie doesn't error, Gemini just treats you as anonymous and plain text requests still return 200
- **Cookies exported from Chrome may not renew**: recent Chrome Device Bound Session Credentials bind the session to the machine, and rotating `__Secure-1PSIDTS` then returns 401. Export from Firefox for a session that can be kept alive
- **Streaming**: Chat and Responses can forward text incrementally. Tool-call fences are held until complete, then returned as structured calls. Reasoning events are available on Chat only

<a id="troubleshooting"></a>

## Troubleshooting

| Symptom | Most likely cause | What to do |
|---|---|---|
| We return **429** | Inspect the error code: `ip_slot_full` means local capacity is exhausted; `model_downgraded` means strict model validation rejected an upstream result | Adjust capacity or proxies for slot errors; inspect account quota and access for model errors |
| Panel diagnostics show **302 → `google.com/sorry/index`** | This exit IP is blocked by Google (after 80-180 requests, depending on connection strategy and exit quality) | Change exit / add proxies. **The block is hard, not probabilistic** (60 probes after a block, zero successes), so retrying in place is pointless |
| Occasional empty responses, logged as an upstream refusal | Upstream transient refusal (`1155`). There is no predictable threshold — it correlates with neither rate, concurrency, nor cumulative count | Resend once and it usually works. **Lowering RPM does not help**: measured, it is unrelated to request rate |
| Every request times out | This host can't reach `gemini.google.com` | Configure a proxy (panel or `--proxy`). Note that **`HTTPS_PROXY` is not read** |
| Exits at startup with `unable to open database file (14)` | The container runs as nonroot (uid 65532) but the bind-mounted host directory is owned by root, so it can't be written | Use a named volume (the default in compose), or `sudo chown -R 65532:65532 ./data` |
| `gemini-3.1-pro` errors out immediately | It isn't exposed without a cookie, by design | Add an account on the Cookie pool page and it becomes available |
| Every request returns 502 after attaching a cookie | The cookie expired, so the XSRF token can't be fetched | Re-export the cookie. Quick check: if `gemini-3.1-pro` reports 3.5 Flash-Lite, it's expired |
| Cookie dies in about 30 minutes; keepalive does nothing | Chrome device-bound session; rotating `__Secure-1PSIDTS` returns 401 | Re-login in **Firefox** and export again. The panel Keep-alive button should report that `__Secure-1PSIDTS` was refreshed |
| Panel won't open / 401 | `--admin-token` (or `ADMIN_TOKEN`) doesn't match | An empty token disables auth, which is only acceptable when bound to 127.0.0.1 |

With Docker's default bridge network, upstream may return empty content (Google rejects certain NAT ranges). We have **never reproduced it here**. If you hit it, try `network_mode: host` to confirm whether that is the cause.

<a id="faq"></a>

## FAQ

### How is this different from the official Gemini API? Do I need an API key?

No API key. This project does not wrap `generativelanguage.googleapis.com`; it reverse-proxies the protocol of the
gemini.google.com web app itself, so it needs no Google API key or official API billing quota. Web-app account access and usage limits still apply. Capabilities follow the implemented
web app: what the web app can do (web search, image, music, canvas, video generation) works here; what it can't
(deep research) doesn't. Anonymous use — no cookie at all — already gives you the text models.

### How does this relate to Sophomoresty/gemini-web2api (Python) and other Go projects named gemini-web2api?

This repository is based on [zexadev/gemini-web2api-go](https://github.com/zexadev/gemini-web2api-go). It retains the Go web-protocol proxy and changes the project layout, model checks, quota queries and admin frontend.
The upstream project's early protocol work drew on [Sophomoresty/gemini-web2api](https://github.com/Sophomoresty/gemini-web2api); this repository is not a port of that Python project.

### Why does my cookie die after about 30 minutes? How do I keep it alive?

The ticket that expires is `__Secure-1PSIDTS` (roughly 30 minutes). The proxy re-mints it on a timer with a sentinel
payload and runs a keepalive, so **Firefox avoids Chrome's device-binding issue, but no session lifetime is guaranteed**. Recent Chrome enables
Device Bound Session Credentials: a session exported from Chrome returns 401 on rotation and dies within 30 minutes
to a few hours. Export from Firefox instead — no browser extension or always-on machine needed.

### Will my account get banned? Is the limit per account or per IP?

Per **exit IP + TLS fingerprint**, not per account. Measured: a burst of roughly 80–180 requests from one exit
triggers the sorry-page redirect, which clears itself after about two hours; a steady pace (e.g. 10 requests per
minute) ran 800 requests without a block. A paid plan (Google AI Pro) does **not** raise this ceiling. To scale,
spread requests across exits with the proxy pool rather than adding accounts.

### What do I get with a free account versus Pro / paid?

Anonymous (no cookie): `gemini-3.6-flash` and `gemini-3.5-flash-lite`, web search included.
A free account's cookie additionally unlocks `gemini-3.1-pro`, available `-thinking` variants,
image / video input, image generation, music and canvas.
Paid accounts only: `gemini-3.8-flash` (free accounts are downgraded to 3.5 Flash-Lite) and video generation
(`gemini-video` / `/v1/videos`).

### Why does every message create a new conversation on gemini.google.com?

Each request is its own conversation by default. Enable **server-side multi-turn** (`multi_turn`) on the panel's
Settings page and consecutive messages of one conversation reuse the same web session, sending only the newest
turn while history stays server-side. To stop conversations piling up in the web app, you have two options:
enable **temporary chat** (`temporary_chat`) so the conversation never enters the list at all (cleaner, not used
for training, nothing to delete — recommended), or enable **auto-delete web conversation**
(`auto_delete_conversation`) to delete it after each reply. Both only matter for signed-in accounts.

### Can I use it as the backend for Cursor / Dify / WorkBuddy-style agents?

You can connect them (it is OpenAI-compatible), and plain chat and coding work fine. But the web app caps a single
request at roughly **130,000 bytes** (about 160,000 with a cookie); agent tools that push tens of kilobytes of system
prompt and history per request will get off-topic answers and look "dumb" — a limit of the web model itself, not of
the proxy. Function calling is prompt-level, so the model does not always answer in the expected format.

### Is gemini-3.8-flash available?

Yes: `gemini-3.8-flash` and `gemini-3.8-flash-thinking`, for paid Gemini accounts; requests from free accounts are
downgraded server-side to 3.5 Flash-Lite. `gemini-3.7-flash` remains as an alias so older clients keep working.

### Which model does image generation use? Can it output 2K?

`gemini-image` uses the web app's image-generation tool slot rather than a pinned model: whatever backend the web
app currently uses (e.g. Nano Banana 2) is what you get, automatically. Resolution is capped by the web app at
roughly one megapixel; there is no 2K.

### Why does streaming turn into a single response behind Dify / a reverse proxy?

Chat and Responses forward text incrementally. With tools, the fence gate buffers tool-call text until it can be parsed while allowing other text through. Responses sends no reasoning events. If a reverse proxy buffers the stream despite `X-Accel-Buffering: no`, check its own buffering settings.

### Does it support MySQL / PostgreSQL?

Yes. Set `SQL_DSN` (`mysql://user:pass@host:3306/db` or `postgres://user:pass@host:5432/db?sslmode=disable`);
unset means SQLite. Tables are created automatically and all three share one schema. SQLite is the easy choice for a
single instance; MySQL / PostgreSQL only matter when several instances share one pool.

---

Read more: [Deployment](deployment.md) · [API and models](api.md) · [Accounts and proxies](operations.md) · [Troubleshooting](troubleshooting.md) · [Development](development.md)
