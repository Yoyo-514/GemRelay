# API, models and clients

[Home](README.md) · [中文](../api.md)

Run commands from the repository root.

## What this is

It turns a call like this:
```
[OpenAI SDK / Cherry Studio / Cursor / dify / newapi / ...]
    ↓ http://localhost:8083/v1/chat/completions
[gemini-web2api-go]
    ↓ reverse-engineered gemini.google.com web protocol
[Google Gemini web app]
```

The service proxies the browser protocol, not Google's official API ([generativelanguage.googleapis.com](https://generativelanguage.googleapis.com)). It needs no Google API key or official API billing quota; web-app account access and usage limits still apply.

## Request examples

### Making a call

```bash
curl http://localhost:8083/v1/chat/completions \
  -H "Authorization: Bearer sk-gemini-..." \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-3.6-flash",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

The OpenAI Python SDK works as-is:

```python
from openai import OpenAI
client = OpenAI(
    base_url="http://localhost:8083/v1",
    api_key="sk-gemini-..."  # find it in the admin panel
)
resp = client.chat.completions.create(
    model="gemini-3.6-flash",
    messages=[{"role": "user", "content": "Explain quantum entanglement"}]
)
print(resp.choices[0].message.content)
```

On Windows PowerShell, `curl` is an alias for `Invoke-WebRequest` and will reinterpret the JSON quoting, so use `curl.exe` with `--%`:

```powershell
curl.exe --% http://127.0.0.1:8083/v1/chat/completions -H "Content-Type: application/json" -H "Authorization: Bearer sk-gemini-..." -d "{\"model\":\"gemini-3.6-flash\",\"messages\":[{\"role\":\"user\",\"content\":\"Hello!\"}]}"
```

<a id="clients"></a>

## Connecting clients

Every OpenAI-compatible client (Cherry Studio / ChatBox / Open WebUI / dify / Cursor / …) is configured the same way:

| Field | Value |
|---|---|
| Base URL | `http://localhost:8083/v1` (some clients want just `http://localhost:8083` and append `/v1` themselves) |
| API key | the `sk-gemini-…` shown on the admin panel's Settings page |
| Model | `gemini-3.6-flash` |

**newapi / one-api channel**: pick the OpenAI type, set the base URL to `http://localhost:8083` (use `http://host.docker.internal:8083` or the container name if newapi runs in Docker too), paste the API key, and list the models as `gemini-3.6-flash,gemini-3.5-flash-lite`.

**Codex CLI** uses `/v1/responses`; set its base URL to `http://localhost:8083/v1`. Text streams incrementally. Tool calls are returned after complete parsing; see [protocol coverage](#protocol).

**Gemini CLI cannot connect.** It expects Google's native `/v1beta/models/{model}:generateContent`; this project only exposes OpenAI-shaped endpoints and does not implement `/v1beta`.

There is also an unauthenticated health check at `GET /` returning `{"status":"ok","version":…,"models":[…]}`.

<a id="mcp"></a>

## MCP (web_search tool)

Besides the OpenAI API, the same process and port also serves an **MCP server** at `/mcp`,
exposing Gemini's web-grounded **search** as a `web_search` tool. This lets MCP clients
(Claude Desktop / Claude Code / Cursor) "search the web through Gemini" and get a
**synthesized answer plus source links** back.

No separate process or deployment — running the backend gives you this. The transport is
HTTP (Streamable HTTP), so remote clients just point at the URL, reusing the backend's
account pool / proxy pool / rate limiting. Search works anonymously; no cookie required.

**Client config** (Claude Desktop's `claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "gemini-search": {
      "url": "http://your-server:8083/mcp",
      "headers": { "Authorization": "Bearer sk-gemini-your-key" }
    }
  }
}
```

- `url` points at the backend's `/mcp`; locally it's `http://localhost:8083/mcp`.
- `Authorization` is the same API key as the OpenAI endpoint (shown on the Settings page).
- Claude Code: `claude mcp add --transport http gemini-search http://localhost:8083/mcp --header "Authorization: Bearer sk-gemini-your-key"`.

The `web_search(query)` tool takes a query/question and returns Gemini's web-grounded
answer with a `Sources:` list appended. It's the only tool for now (`url_context` — reading
a specific URL's content — is not implemented yet).

<a id="models"></a>

## Models

Model parameters follow the web catalog (`batchexecute?rpcids=otAQ7b`). Registered models are listed below; account access and upstream quota still apply:

| Model | Description |
|---|---|
| `gemini-3.6-flash` | All-round, default |
| `gemini-3.5-flash-lite` | Fast and lightweight |
| `gemini-3.1-pro` | Most capable, **needs a cookie**; every reply carries a reasoning chain |
| `gemini-3.8-flash` | Newer Flash, **needs a cookie from a PAID Gemini account** (free accounts get downgraded to 3.5 Flash-Lite). `gemini-3.7-flash` is an alias — Google bumped that same hex from 3.7 to 3.8 in place |
| `gemini-4-argon` | Argon, low thinking by default; requires a signed-in account with access |
| `gemini-4-argon-low` / `gemini-4-argon-medium` / `gemini-4-argon-high` | Low / medium / high tiers sharing one model identifier; account access required |
| `gemini-3.6-flash-thinking` | 3.6 Flash with extended thinking; **needs a cookie** |
| `gemini-3.5-flash-lite-thinking` | 3.5 Flash-Lite with extended thinking; **needs a cookie** |
| `gemini-3.1-pro-thinking` | 3.1 Pro with extended thinking; **needs a cookie** |
| `gemini-3.8-flash-thinking` | 3.8 Flash with extended thinking; **needs a paid-account cookie** (`gemini-3.7-flash-thinking` is an alias) |
| `gemini-image` | Image generation (Nano Banana); base64 output; **needs a cookie** |
| `gemini-music` | Music (Lyria, ~30s); base64 output; **needs a cookie** |
| `gemini-canvas` | Canvas: generates an interactive HTML document (returned inline as a ```html block); **needs a cookie** |
| `gemini-video` | Video generation (async, tens of seconds to a few minutes); base64 MP4 output; **needs a Pro/paid account** (free accounts are refused by the upstream content policy) |

Without a cookie, `/v1/models` returns only the first two, and asking for `gemini-3.1-pro` fails with an explanation. An anonymous request for it is always silently downgraded to 3.5 Flash-Lite — better to fail at model selection than to hand back a reply that "succeeded" but isn't Pro.

The upstream project recorded six consecutive calls reporting `3.1 Pro` with a valid cookie. Access and quota can change. Inspect the actual model in request details or enable strict model checks.

Only registered models and aliases are exposed. The old names `gemini-3.5-flash`, `gemini-3.5-flash-thinking`, `gemini-3.5-flash-thinking-lite`, `gemini-auto` and `gemini-flash-lite` were **removed** (they now return 400): the backend has no entries for them, and keeping them only suggested there were five distinct models to choose from.

> **`@think=N` is deprecated.** Captures identify `inner[17]` as the conversation turn index, not thinking depth. Single-turn requests write 0; server-side continuation increments it. The suffix is accepted and ignored. Use the registered thinking variants or Argon tiers instead.

<a id="reasoning"></a>

### Reasoning chain (`reasoning_content`)

When upstream provides reasoning, Chat Completions exposes it as `reasoning_content`. Availability depends on model settings and account access; absent reasoning is omitted. Responses currently does not emit reasoning events.

Non-streaming:

```json
{"choices":[{"message":{
  "role":"assistant",
  "content":"The core rule of this classic puzzle is...",
  "reasoning_content":"**Defining the Constraints**\n\nI've successfully defined..."
}}],
 "usage":{"prompt_tokens":26,"completion_tokens":337,"reasoning_tokens":77,"total_tokens":363}}
```

Streaming sends it as `delta.reasoning_content`, and **the whole reasoning chain is
streamed before any answer text** — that is the order the upstream itself uses, so the
experience matches the Gemini web UI.

`reasoning_tokens` is counted **separately and is not included in `completion_tokens`**:
clients collapse the reasoning by default, so billing it would charge users for output
they never see (newapi bills on `completion_tokens`). Add the two together yourself if
you do want to bill for it.

### Known capability boundaries

Anonymous calls (no cookie) only reach the two text models above plus Gemini's built-in web search. `gemini-3.1-pro` is silently downgraded to 3.5 Flash-Lite anonymously, which is why it isn't exposed at all in that case.

Attaching a cookie additionally unlocks `gemini-3.1-pro`, **extended-thinking variants and Argon when the account has access**, **image / video input**, **a longer context** (over-long conversations are sent as a text attachment, see below), and **image generation (`gemini-image`), music (`gemini-music`), canvas (`gemini-canvas`), video generation (`gemini-video`, needs a Pro account)**.

Deep research is still **not implemented** (a multi-step async flow). The panel's "actual model" column always shows which model the backend really used, so any downgrade is visible.

### Image & music

`gemini-image` (Nano Banana) and `gemini-music` (Lyria, ~30s) go through `/v1/chat/completions` just like a normal chat — put the image/music description in the user message. The bytes come back as a **base64 data URL** inside `content`: images as `![image](data:image/png;base64,…)`, audio as `[audio](data:audio/mpeg;base64,…)`. Markdown-capable clients render the image inline; to save a file, decode the base64 after the comma.

```bash
curl http://127.0.0.1:8083/v1/chat/completions \
  -H "Authorization: Bearer sk-gemini-..." -H "Content-Type: application/json" \
  -d '{"model":"gemini-image","messages":[{"role":"user","content":"an orange cat in an astronaut helmet"}]}'
```

Not returning a URL is deliberate — Gemini's artifact links need the cookie to download, so a bare link would be dead on the client side; the server fetches the bytes and inlines them. The base64 is **not counted in `completion_tokens`** (a single image is millions of tokens; billing that downstream would be absurd), only the caption text is. Both need a cookie, so they don't appear in `/v1/models` without one. `size` has no upstream control and is ignored; `n > 1` returns 400.

The default single-turn path flattens `messages` into one prompt. Enabling `multi_turn` allows the server to reuse a Gemini conversation and send only the newest message on a matching continuation. A new conversation or the attachment path still sends full history. The upstream request-body limit was measured at about **130,000 UTF-8 bytes**; without local checks, the upstream can truncate from the tail, where the latest message sits.

With a cookie, an over-long conversation is uploaded as a `message.txt` attachment instead, which gets around the request-body wall — **but the attachment has a wall of its own**: the model only ever sees about **160,000 bytes** of content in total; anything past that is uploaded but never read (measured with the total size held fixed and only the marker's offset moved: readable at 157,833, not readable at 163,371; splitting into several attachments does not raise the budget). So a cookie takes the usable length from 130K to about 160K — **a genuinely long conversation still has to be compacted by the client**. Without a cookie the request is rejected with 400 `context_length_exceeded` rather than losing data silently.

<a id="protocol"></a>

## OpenAI protocol coverage

| Endpoint | Status | Notes |
|---|---|---|
| `POST /v1/chat/completions` | Supported | Incremental text and reasoning. With tools, text outside tool-call fences streams; structured tool calls follow complete parsing. Successful streams end with `finish_reason` and `[DONE]`; failures use an error chunk |
| `POST /v1/responses` | Supported | Incremental text with Responses event ordering; tool calls follow complete parsing. No reasoning events. Failures end with `response.failed`, not `response.completed` |
| `GET /v1/models` | Supported | Two text models anonymously; an enabled account exposes registered signed-in models. Listing a model does not prove that the account has upstream access |
| `POST /v1/videos`, `GET /v1/videos/{id}`, `GET /v1/videos/{id}/content` | Supported | Create a video job, inspect its state and download MP4; account access required |
| `POST /mcp` | Supported | Streamable HTTP `web_search`, authenticated with the API key |
| `GET /` | ✅ | Health check, unauthenticated, returns status/version/models |
| `/v1beta/models/…` (Gemini CLI native) | ❌ | Not implemented, only OpenAI-shaped endpoints are exposed |
| `/v1/embeddings`, `/v1/images/*`, `/v1/audio/*` | ❌ | Not implemented, return 404 |
| Function calling | ⚠️ | Prompt-level implementation (the model emits a ` ```tool_call``` ` block that we parse with a regex), not a real protocol layer. **Reliable for looking up private data or internal systems**, but anything Gemini can answer itself (the weather) gets answered directly, and side-effecting actions (sending email) are refused. **Agentic clients (Codex, etc.) now work**: before 4.11.0 their tens-of-KB system prompts buried our tool instructions and caused off-topic replies; fixed now. Since 4.12.0 tool results are condensed into a clear success/failure signal, so weak models no longer misread a no-output success as a failure and loop retrying (measured: the same task dropped from 26 loops to 2-4 before it concludes) |
| `tool_choice` | ⚠️ | `none` injects no tool definitions at all; `required` and a named function add mandatory wording and drop the other tools from the prompt. A prompt-level layer **cannot truly force it** — in testing, questions the model can answer itself (weather, 2+2) were answered directly even under `required` |
| `stream_options.include_usage` | ✅ | Emits a usage chunk with an empty `choices` array after `finish_reason` |
| `usage` token counts | ✅ | tiktoken cl100k_base, the same basis as the panel's requests table |
| `n` > 1 | ❌ | Returns 400. Upstream yields a single candidate; silently treating it as 1 would short-change the client |
| Sampling params | ➖ | `temperature` / `top_p` / `max_tokens` / `stop` / `seed` / `presence_penalty` / `frequency_penalty` are **accepted and ignored, not rejected**. Gemini's web protocol has no such knobs |
| `response_format` / `logprobs` | ➖ | Not implemented, accepted and ignored |
| Vision / image input | ⚠️ | **Works with a cookie**: both `image_url` (chat) and `input_image` (responses) are accepted, from `data:` URLs or http(s) links, up to 12MB per image. Anonymous returns 400 — anonymous upload **does** work (`content-push.googleapis.com/upload/`, two-step resumable), but referencing the uploaded file in a conversation is refused upstream (`BardErrorInfo 1100`) |
| Audio | ❌ | Sending `input_audio` returns 400. The web app has music generation (Lyria 3), signed-in only |

---

Read more: [Deployment](deployment.md) · [API and models](api.md) · [Accounts and proxies](operations.md) · [Troubleshooting](troubleshooting.md) · [Development](development.md)
