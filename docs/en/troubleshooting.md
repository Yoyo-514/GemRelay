# Limitations and troubleshooting

[Home](README.md) · [中文](../troubleshooting.md)

Run commands from the repository root.

<a id="limitations"></a>

## Limitations

- **Per-IP ceiling**: burst tests hit the sorry page after about **80-180 requests**; a static IP completed 800 requests at 10/minute. The default `per_ip_rph=80` takes the low end of the burst range; adjust it based on exit behavior and pacing. See [proxy measurements](operations.md#proxies)
- **Signed-in features**: image, music, canvas and video generation are implemented; video needs account access. Deep research's multi-step async flow is currently unimplemented
- **Function calling**: prompts define the output format, so reliability depends on model compliance
- **Multimodal**: image/video input requires a cookie. Image, music, canvas and video generation also require a cookie, and video needs a Pro/paid account
- **Long context**: request bodies allow about 130,000 bytes; signed-in attachment uploads raise usable input to about 160,000 bytes of total model-visible content. Splitting attachments leaves that total unchanged. Longer conversations need client-side compaction
- **Token counts**: tiktoken estimates (Gemini's tokenizer is not public), within about ±20% in recorded tests
- **Cookie account state**: results are written back, with failure counts covering 401/403 only; network errors and 302 blocks are classified as exit problems. Accounts are disabled from the panel. `last_ok_at` records request success; expired cookies may be treated as anonymous, so use Check to verify the signed-in session
- **Cookie renewal**: recent Chrome device-bound sessions may return 401 when renewing `__Secure-1PSIDTS`. Prefer Firefox for export; renewal depends on upstream policy
- **Streaming**: Chat and Responses forward text incrementally. Tool-call fences are parsed completely and returned as structured calls. Reasoning increments are available on Chat only

<a id="troubleshooting"></a>

## Troubleshooting

| Symptom | Most likely cause | What to do |
|---|---|---|
| Service returns **429** | `ip_slot_full` means local capacity is exhausted; `model_downgraded` means strict checks found a model mismatch | Adjust limits or proxies for slot errors; inspect account quota and access for model errors |
| Panel diagnostics show **302 → `google.com/sorry/index`** | Google risk controls have restricted this exit IP | Change exit or wait for recovery. All 60 probes after blocking failed; see [proxy measurements](operations.md#proxies) |
| Occasional empty responses, logged as an upstream refusal | Transient upstream refusal (`1155`); historical tests found no correlation with rate, concurrency or cumulative count | Retry once; lowering RPM was ineffective in those tests |
| Every request times out | Connection from this host to `gemini.google.com` fails | Configure a proxy in the panel or through `--proxy`; `HTTPS_PROXY` is ignored |
| Exits at startup with `unable to open database file (14)` | The container runs as nonroot (uid 65532) but the bind-mounted host directory is owned by root, so it can't be written | Use a named volume (the default in compose), or `sudo chown -R 65532:65532 ./data` |
| `gemini-3.1-pro` errors out immediately | The model requires a signed-in account | Import an account with the required access on the Cookie pool page |
| Every request returns 502 after attaching a cookie | The cookie expired, so the XSRF token can't be fetched | Re-export the cookie. Quick check: if `gemini-3.1-pro` reports 3.5 Flash-Lite, it's expired |
| Cookie dies in about 30 minutes; keepalive does nothing | Chrome device-bound session; rotating `__Secure-1PSIDTS` returns 401 | Re-login in **Firefox** and export again. The panel Keep-alive button should report that `__Secure-1PSIDTS` was refreshed |
| Panel fails to open / 401 | The supplied token differs from `--admin-token` or `ADMIN_TOKEN` | Check the token. Leaving it empty disables authentication and is suitable only for local deployments bound to 127.0.0.1 |

There are reports of empty upstream responses under Docker's default bridge network, possibly due to NAT exits being rejected by Google. This remains unreproduced in this project. Compare with `network_mode: host` to check whether network mode is involved.

<a id="faq"></a>

## FAQ

### How is this different from the official Gemini API? Do I need an API key?

Requests use the gemini.google.com web protocol and need no Google API key or official API billing quota.
Account access and quota follow web-app rules. Implemented features include web search, image, music,
canvas and video generation. Two text models support anonymous use.

### How does this relate to Sophomoresty/gemini-web2api (Python) and other Go projects named gemini-web2api?

This repository is based on [zexadev/gemini-web2api-go](https://github.com/zexadev/gemini-web2api-go). It retains the Go web-protocol proxy and changes the project layout, model checks, quota queries and admin frontend.
Early protocol work drew on [Sophomoresty/gemini-web2api](https://github.com/Sophomoresty/gemini-web2api); this repository follows the Go implementation.

### Why does my cookie die after about 30 minutes? How do I keep it alive?

`__Secure-1PSIDTS` lasts about 30 minutes. The service periodically renews it with a sentinel payload and runs keepalive.
Recent Chrome Device Bound Session Credentials may cause exported cookies to return 401 during renewal
and expire within 30 minutes to a few hours. Prefer Firefox for export and let the service handle keepalive;
actual session lifetime depends on upstream policy. See [cookie renewal](operations.md#cookies).

### Will my account get banned? Is the limit per account or per IP?

Observed sorry-page blocks relate to exit IP and TLS fingerprint; paid accounts
(Google AI Pro) face the same class of exit restrictions. See [proxy measurements](operations.md#proxies)
for burst and recovery records. The proxy pool distributes exits; the account pool handles account
rotation and access selection.

### What do I get with a free account versus Pro / paid?

Anonymous (no cookie): `gemini-3.6-flash` and `gemini-3.5-flash-lite`, web search included.
A free account's cookie additionally unlocks `gemini-3.1-pro`, available `-thinking` variants,
image / video input, image generation, music and canvas.
Paid accounts only: `gemini-3.8-flash` (free accounts are downgraded to 3.5 Flash-Lite) and video generation
(`gemini-video` / `/v1/videos`).

### Why does every message create a new conversation on gemini.google.com?

Each request is its own conversation by default. Enable **server-side multi-turn** (`multi_turn`) on the panel's
Settings page and consecutive messages of one conversation reuse the same web session, sending only the newest
turn while history stays server-side. For signed-in single-turn requests, choose **temporary chat**
(`temporary_chat`) to exclude conversations from the account list and model training, or **auto-delete
web conversation** (`auto_delete_conversation`) to attempt deletion after successful generation.

### Can I use it as the backend for Cursor / Dify / WorkBuddy-style agents?

You can connect them (it is OpenAI-compatible), and plain chat and coding work. The web app caps a single
request at roughly **130,000 bytes** (about 160,000 with a cookie); agent requests with large system prompts
and history may hit that limit and get affected answers. Function calling depends on the model following
prompt conventions, so verify behavior with your actual tasks.

### Is gemini-3.8-flash available?

Yes: `gemini-3.8-flash` and `gemini-3.8-flash-thinking`, for paid Gemini accounts; requests from free accounts are
downgraded server-side to 3.5 Flash-Lite. `gemini-3.7-flash` remains as an alias so older clients keep working.

### Which model does image generation use? Can it output 2K?

`gemini-image` uses the web app's image-generation tool slot, following its current backend, such as Nano Banana 2.
The current resolution cap is about one megapixel; 2K output is unavailable.

### Why does streaming turn into a single response behind Dify / a reverse proxy?

Chat and Responses forward text incrementally. With tools, the fence gate holds tool-call text for parsing while forwarding other text. Reasoning events are available on Chat only. If a reverse proxy buffers the stream despite `X-Accel-Buffering: no`, check its buffering settings.

### Does it support MySQL / PostgreSQL?

Yes. Set `SQL_DSN` (`mysql://user:pass@host:3306/db` or `postgres://user:pass@host:5432/db?sslmode=disable`);
SQLite is the default. Tables are created automatically and all three share one schema. SQLite suits a single
instance; MySQL / PostgreSQL can serve several instances sharing pool data.

---

Read more: [Deployment](deployment.md) · [API and models](api.md) · [Accounts and proxies](operations.md) · [Troubleshooting](troubleshooting.md) · [Development](development.md)
