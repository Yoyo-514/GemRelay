# API, models and clients

[Home](README.md) · [中文](../api.md)

Run commands from the repository root.

## What this is

Requests follow this path:
```
[OpenAI SDK / Cherry Studio / Cursor / dify / newapi / ...]
    ↓ http://localhost:8083/v1/chat/completions
[GemRelay]
    ↓ reverse-engineered gemini.google.com web protocol
[Google Gemini web app]
```

The service uses Gemini's web protocol, independently of Google's official API ([generativelanguage.googleapis.com](https://generativelanguage.googleapis.com)). Requests use anonymous access or account cookies, with web-app quota rules.

## Request examples

### Making a call

```bash
curl http://localhost:8083/v1/chat/completions \
  -H "Authorization: Bearer sk-gemrelay-..." \
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
    api_key="sk-gemrelay-..."  # find it in the admin panel
)
resp = client.chat.completions.create(
    model="gemini-3.6-flash",
    messages=[{"role": "user", "content": "Explain quantum entanglement"}]
)
print(resp.choices[0].message.content)
```

On Windows PowerShell, `curl` is an alias for `Invoke-WebRequest` and will reinterpret the JSON quoting, so use `curl.exe` with `--%`:

```powershell
curl.exe --% http://127.0.0.1:8083/v1/chat/completions -H "Content-Type: application/json" -H "Authorization: Bearer sk-gemrelay-..." -d "{\"model\":\"gemini-3.6-flash\",\"messages\":[{\"role\":\"user\",\"content\":\"Hello!\"}]}"
```

<a id="clients"></a>

## Connecting clients

Every OpenAI-compatible client (Cherry Studio / ChatBox / Open WebUI / dify / Cursor / …) is configured the same way:

| Field | Value |
|---|---|
| Base URL | `http://localhost:8083/v1` (some clients want just `http://localhost:8083` and append `/v1` themselves) |
| API key | the `sk-gemrelay-…` shown on the admin panel's Settings page |
| Model | `gemini-3.6-flash` |

**newapi / one-api channel**: pick the OpenAI type, set the base URL to `http://localhost:8083` (use `http://host.docker.internal:8083` or the container name if newapi runs in Docker too), paste the API key, and list the models as `gemini-3.6-flash,gemini-3.5-flash-lite`.

**Codex CLI** uses `/v1/responses`; set its base URL to `http://localhost:8083/v1`. Text streams incrementally. Tool calls are returned after complete parsing; see [protocol coverage](#protocol).

**Gemini CLI** expects Google's native `/v1beta/models/{model}:generateContent` format. That differs from this service's OpenAI-compatible API, so Gemini CLI is currently unsupported.

There is also an unauthenticated health check at `GET /` returning `{"status":"ok","version":…,"models":[…]}`.

<a id="mcp"></a>

## MCP (web_search tool)

The same process and port serve an MCP server at `/mcp`, exposing Gemini's web search as
the `web_search` tool. MCP clients such as Claude Desktop, Claude Code and Cursor receive
a synthesized answer and source links.

MCP starts with the backend, uses Streamable HTTP, shares the account pool, proxy pool and
rate limits, and supports anonymous search.

**Client config** (Claude Desktop's `claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "gemini-search": {
      "url": "http://your-server:8083/mcp",
      "headers": { "Authorization": "Bearer sk-gemrelay-your-key" }
    }
  }
}
```

- `url` points at the backend's `/mcp`; locally it's `http://localhost:8083/mcp`.
- `Authorization` is the same API key as the OpenAI endpoint (shown on the Settings page).
- Claude Code: `claude mcp add --transport http gemini-search http://localhost:8083/mcp --header "Authorization: Bearer sk-gemrelay-your-key"`.

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
| `gemini-3.1-pro` | Pro model, requires a cookie; every reply in tests included reasoning |
| `gemini-3.8-flash` | Requires a paid Gemini account cookie; free accounts are downgraded to 3.5 Flash-Lite. `gemini-3.7-flash` is an alias sharing the hex identifier that upstream upgraded to 3.8 |
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

For anonymous requests, `/v1/models` returns the first two models, and `gemini-3.1-pro` fails at model selection. This catches the case where upstream would downgrade an anonymous Pro request to 3.5 Flash-Lite.

Tests recorded six consecutive calls reporting `3.1 Pro` with a valid cookie. Access and quota can change. Inspect the actual model in request details or enable strict model checks.

The API accepts registered models and aliases. The old names `gemini-3.5-flash`, `gemini-3.5-flash-thinking`, `gemini-3.5-flash-thinking-lite`, `gemini-auto` and `gemini-flash-lite` were removed with their upstream entries; requests using them return 400.

> `@think=N` is deprecated and is accepted but ignored. Captures identify `inner[17]` as the conversation turn index: single-turn requests write 0, and server-side continuation increments it. Use registered thinking variants or Argon tiers to set thinking depth.

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

Streaming sends reasoning as `delta.reasoning_content`, followed by answer text, matching
the upstream web app's order.

`reasoning_tokens` and `completion_tokens` are counted separately. Clients usually collapse
reasoning, and downstream services such as newapi bill on `completion_tokens`. Add the two
counts if reasoning should be included in billing.

### Known capability boundaries

Signed-in requests can use `gemini-3.1-pro`, **extended-thinking variants and Argon when the account has access**, **image / video input**, **a longer context** (over-long conversations are sent as a text attachment, see below), and **image generation (`gemini-image`), music (`gemini-music`), canvas (`gemini-canvas`), video generation (`gemini-video`, needs a Pro account)**.

Deep research's multi-step async flow is currently unimplemented. The panel's "actual model" column shows the model reported by upstream to help identify downgrades.

### Image & music

`gemini-image` (Nano Banana) and `gemini-music` (Lyria, ~30s) go through `/v1/chat/completions` just like a normal chat — put the image/music description in the user message. The bytes come back as a **base64 data URL** inside `content`: images as `![image](data:image/png;base64,…)`, audio as `[audio](data:audio/mpeg;base64,…)`. Markdown-capable clients render the image inline; to save a file, decode the base64 after the comma.

```bash
curl http://127.0.0.1:8083/v1/chat/completions \
  -H "Authorization: Bearer sk-gemrelay-..." -H "Content-Type: application/json" \
  -d '{"model":"gemini-image","messages":[{"role":"user","content":"an orange cat in an astronaut helmet"}]}'
```

Gemini artifact links require cookies, so the server downloads the bytes and returns them as base64. `completion_tokens` counts caption text only, excluding image and audio data. Both models require a signed-in session and appear in the signed-in model list. `size` is accepted and ignored; `n > 1` returns 400.

The default single-turn path flattens `messages` into one prompt. Enabling `multi_turn` allows the server to reuse a Gemini conversation and send only the newest message on a matching continuation. A new conversation or the attachment path still sends full history and remains subject to the request-body limit, measured at about **130,000 UTF-8 bytes**. Local checks catch oversized inputs before upstream can truncate the tail containing the latest message.

For signed-in requests, over-long conversations are uploaded as a `message.txt` attachment, raising the usable length from about 130,000 to **160,000 bytes**. The attachment limit applies to total model-visible content; uploaded data beyond it may be unreadable. With total size fixed and only the marker offset changed, the marker was readable at 157,833 bytes but not at 163,371. Splitting the content across attachments left the total budget unchanged. Longer conversations need client-side compaction. Oversized anonymous requests return 400 `context_length_exceeded`.

<a id="protocol"></a>

## OpenAI protocol coverage

| Endpoint | Status | Notes |
|---|---|---|
| `POST /v1/chat/completions` | Supported | Incremental text and reasoning. With tools, text outside tool-call fences streams; structured tool calls follow complete parsing. Successful streams end with `finish_reason` and `[DONE]`; failures use an error chunk |
| `POST /v1/responses` | Supported | Incremental text with Responses event ordering; reasoning events are currently unavailable. Tool calls follow complete parsing. Success ends with `response.completed`, failure with `response.failed` |
| `GET /v1/models` | Supported | Two text models anonymously; an enabled account exposes registered signed-in models. Using them still requires the account's upstream access |
| `POST /v1/videos`, `GET /v1/videos/{id}`, `GET /v1/videos/{id}/content` | Supported | Create a video job, inspect its state and download MP4; account access required |
| `POST /mcp` | Supported | Streamable HTTP `web_search`, authenticated with the API key |
| `GET /` | ✅ | Health check, unauthenticated, returns status/version/models |
| `/v1beta/models/…` (Gemini CLI native) | ❌ | Not implemented, only OpenAI-shaped endpoints are exposed |
| `/v1/embeddings`, `/v1/images/*`, `/v1/audio/*` | ❌ | Not implemented, return 404 |
| Function calling | ⚠️ | Prompts ask the model to emit a ` ```tool_call``` ` block, which is parsed with a regex. Private-data and internal-system lookups are relatively reliable; the model may answer weather questions itself or refuse side-effecting actions such as email. Agent clients including Codex can connect. Release 4.11.0 fixed long system prompts burying tool instructions; 4.12.0 condensed tool results into success/failure signals to reduce retries caused by misreading empty output as failure. One test task dropped from 26 turns to 2–4 |
| `tool_choice` | ⚠️ | `none` skips tool definitions; `required` strengthens the instruction, and a named function keeps only that tool. Compliance depends on the model following the prompt: weather and 2+2 questions were still answered directly under `required` in tests |
| `stream_options.include_usage` | ✅ | Emits a usage chunk with an empty `choices` array after `finish_reason` |
| `usage` token counts | ✅ | tiktoken cl100k_base, the same basis as the panel's requests table |
| `n` > 1 | ❌ | Returns 400; upstream provides a single candidate |
| Sampling params | ➖ | `temperature` / `top_p` / `max_tokens` / `stop` / `seed` / `presence_penalty` / `frequency_penalty` are accepted and ignored; the web protocol lacks corresponding controls |
| `response_format` / `logprobs` | ➖ | Not implemented, accepted and ignored |
| Vision / image input | ⚠️ | **Works with a cookie**: both `image_url` (chat) and `input_image` (responses) are accepted, from `data:` URLs or http(s) links, up to 12MB per image. Anonymous returns 400 — anonymous upload **does** work (`content-push.googleapis.com/upload/`, two-step resumable), but referencing the uploaded file in a conversation is refused upstream (`BardErrorInfo 1100`) |
| Audio | ❌ | Sending `input_audio` returns 400. The web app has music generation (Lyria 3), signed-in only |

---

Read more: [Deployment](deployment.md) · [API and models](api.md) · [Accounts and proxies](operations.md) · [Troubleshooting](troubleshooting.md) · [Development](development.md)
