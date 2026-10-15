# API、模型与客户端

[首页](../README.md) · [English](en/api.md)

命令均从仓库根目录执行。

## 这是什么

请求经过以下路径转发：
```
[OpenAI SDK / Cherry Studio / Cursor / dify / newapi / ...]
    ↓ http://localhost:8083/v1/chat/completions
[GemRelay]
    ↓ 逆向 gemini.google.com 网页协议
[Google Gemini 网页端]
```

本项目通过 Gemini 网页协议访问模型，与 Google 官方 API（[generativelanguage.googleapis.com](https://generativelanguage.googleapis.com)）独立。调用使用匿名请求或账号 Cookie，额度按网页端规则计算。

## 调用示例

### 调用

```bash
curl http://localhost:8083/v1/chat/completions \
  -H "Authorization: Bearer sk-gemrelay-..." \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-3.6-flash",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

OpenAI Python SDK 也直接能用：

```python
from openai import OpenAI
client = OpenAI(
    base_url="http://localhost:8083/v1",
    api_key="sk-gemrelay-..."  # admin 面板里看
)
resp = client.chat.completions.create(
    model="gemini-3.6-flash",
    messages=[{"role": "user", "content": "解释量子纠缠"}]
)
print(resp.choices[0].message.content)
```

Windows PowerShell 下 `curl` 是 `Invoke-WebRequest` 的别名，会把 JSON 引号重新解释，要用
`curl.exe` 加 `--%`：

```powershell
curl.exe --% http://127.0.0.1:8083/v1/chat/completions -H "Content-Type: application/json" -H "Authorization: Bearer sk-gemrelay-..." -d "{\"model\":\"gemini-3.6-flash\",\"messages\":[{\"role\":\"user\",\"content\":\"Hello!\"}]}"
```

<a id="clients"></a>

## 客户端接入

任何 OpenAI 兼容客户端（Cherry Studio / ChatBox / Open WebUI / dify / Cursor / …）都是同一套填法：

| 字段 | 值 |
|---|---|
| Base URL / API 地址 | `http://localhost:8083/v1`（部分客户端只要 `http://localhost:8083`，会自己拼 `/v1`） |
| API Key | 管理面板「设置」页里的那个 `sk-gemrelay-…` |
| 模型 | `gemini-3.6-flash` |

**newapi / one-api 建渠道**：类型选 OpenAI，Base URL 填 `http://localhost:8083`（跟 newapi 同在
docker 里就写 `http://host.docker.internal:8083` 或容器名），密钥填 API key，模型填
`gemini-3.6-flash,gemini-3.5-flash-lite`。

**Codex CLI** 走 `/v1/responses`，把 base url 指到 `http://localhost:8083/v1` 即可（该端点已实现，
支持增量正文；工具调用完整解析后返回，见[协议覆盖](#protocol)）。

**Gemini CLI** 使用 Google 原生的 `/v1beta/models/{model}:generateContent`，与本项目提供的
OpenAI 兼容接口格式不同，暂不支持接入。

健康检查 `GET /` 允许公开访问，返回 `{"status":"ok","version":…,"models":[…]}`。

<a id="mcp"></a>

## MCP（web_search 工具）

服务在同一进程、同一端口的 `/mcp` 提供 MCP server，将 Gemini 网页端的联网搜索
作为 `web_search` 工具供 Claude Desktop / Claude Code / Cursor 等客户端调用，返回合成答案和来源链接。

MCP 随后端一起启动，使用 Streamable HTTP 传输，复用账号池、代理池和限流设置，支持匿名搜索。

**客户端配置**（以 Claude Desktop 的 `claude_desktop_config.json` 为例）：

```json
{
  "mcpServers": {
    "gemini-search": {
      "url": "http://你的服务器:8083/mcp",
      "headers": { "Authorization": "Bearer sk-gemrelay-你的key" }
    }
  }
}
```

- `url` 指向后端的 `/mcp`；本机跑就是 `http://localhost:8083/mcp`。
- `Authorization` 填 OpenAI 接口那把同样的 API key（面板「设置」页里看）。
- Claude Code：`claude mcp add --transport http gemini-search http://localhost:8083/mcp --header "Authorization: Bearer sk-gemrelay-你的key"`。

工具 `web_search(query)`：传一个查询/问题，返回 Gemini 联网查证后的答案，末尾附
`Sources:` 来源清单。当前只有这一个工具（读取指定 URL 的 `url_context` 暂未做）。

<a id="models"></a>

## 模型

模型参数参考网页端清单（`batchexecute?rpcids=otAQ7b`）。当前注册项如下，实际权限和额度由账号及上游决定：

| 模型 | 描述 |
|---|---|
| `gemini-3.6-flash` | 全方位，默认 |
| `gemini-3.5-flash-lite` | 极速、轻量 |
| `gemini-3.1-pro` | Pro 模型，需要 Cookie；测试中每次回答均带思考链 |
| `gemini-3.8-flash` | 需要付费 Gemini 账号 Cookie，免费号会降级为 3.5 Flash-Lite。`gemini-3.7-flash` 为别名，两者共用上游已升级到 3.8 的 hex 标识 |
| `gemini-4-argon` | Argon，默认低思考档；需要登录账号已开放该模型 |
| `gemini-4-argon-low` / `gemini-4-argon-medium` / `gemini-4-argon-high` | 低 / 中 / 高思考档，共用模型标识，需要账号权限 |
| `gemini-3.6-flash-thinking` | 3.6 Flash 开扩展思考，**要配 cookie** |
| `gemini-3.5-flash-lite-thinking` | 3.5 Flash-Lite 开扩展思考，**要配 cookie** |
| `gemini-3.1-pro-thinking` | 3.1 Pro 开扩展思考，**要配 cookie** |
| `gemini-3.8-flash-thinking` | 3.8 Flash 开扩展思考，**要付费号 cookie**（`gemini-3.7-flash-thinking` 为别名）|
| `gemini-image` | 生图（Nano Banana），产物 base64，**要配 cookie** |
| `gemini-music` | 音乐（Lyria，约 30 秒），产物 base64，**要配 cookie** |
| `gemini-canvas` | 画布，生成交互 HTML 文档（内联返回 ```html 块），**要配 cookie** |
| `gemini-video` | 生视频（异步，几十秒~几分钟），产物 base64 MP4，**要 Pro/付费号**；免费号被上游内容政策拒 |

匿名状态下，`/v1/models` 仅返回前两个模型，请求 `gemini-3.1-pro` 会在模型选择阶段返回错误。
这是为了提前识别上游将匿名 Pro 请求降级为 3.5 Flash-Lite 的情况。

测试中曾用有效 Cookie 连续调用 6 次，服务端均回报 `3.1 Pro`。账号权限与额度可能变化，当前结果以请求明细中的实际模型为准，也可开启严格模型校验。

`-thinking` 对应网页 UI 的扩展思考开关，与基础模型标识独立。Argon 使用低 / 中 / 高档位。
部分显示名带 `Extended`（如 `3.6 Flash Extended`），思考级别仍需结合请求设置判断。历史测试记录扩展版输出 2467 / 1059 / 583 字符，普通版为 0 / 0 / 268，仅供参考。
扩展思考在登录态生效，相关档位列入登录模型清单。

接口接受当前注册的模型与别名。旧名称 `gemini-3.5-flash`、`gemini-3.5-flash-thinking`、
`gemini-3.5-flash-thinking-lite`、`gemini-auto`、`gemini-flash-lite` 已随对应服务端条目移除，
请求这些名称返回 400。

> `@think=N` 已废弃，后缀仍接受但忽略。抓包确认 `inner[17]` 表示会话轮次索引：单轮路径写 0，多轮续接路径递增。设置思考级别请使用已注册的思考版模型或 Argon 档位。

<a id="reasoning"></a>

### 思考链（reasoning_content）

上游提供思考链时，Chat Completions 通过 `reasoning_content` 返回。该字段取决于模型设置、账号权限和上游结果，空内容时省略。思考链事件目前仅在 Chat Completions 路径提供。

非流式：

```json
{"choices":[{"message":{
  "role":"assistant",
  "content":"The core rule of this classic puzzle is...",
  "reasoning_content":"**Defining the Constraints**

I've successfully defined..."
}}],
 "usage":{"prompt_tokens":26,"completion_tokens":337,"reasoning_tokens":77,"total_tokens":363}}
```

流式通过 `delta.reasoning_content` 发送思考链，随后发送正文，顺序与上游网页端一致。

`reasoning_tokens` 与 `completion_tokens` 分开统计。客户端通常折叠思考链，newapi 等下游按
`completion_tokens` 计费；需要将思考链纳入计费时，可将两者相加。

### 已知的能力边界

登录后可使用：`gemini-3.1-pro`、**账号有权使用的扩展思考版与 Argon**、**读图 / 读视频**、
**更长的上下文**（超长对话自动转成文本附件发，见下文），以及 **生图（`gemini-image`）**、
**音乐（`gemini-music`）**、**画布（`gemini-canvas`）**、**生视频（`gemini-video`，要 Pro 号）**。

深度研究的多步异步流程暂未实现。管理面板的「实际模型」列显示上游回报的模型，供检查降级情况。

### 生图 / 音乐

`gemini-image`（Nano Banana）和 `gemini-music`（Lyria，约 30 秒）跟普通对话一样走
`/v1/chat/completions`，user 消息里写要画什么 / 要什么曲子。产物字节以 **base64
data URL** 放进返回的 `content`：图片是 `![image](data:image/png;base64,…)`、音频是
`[audio](data:audio/mpeg;base64,…)`。支持 markdown 的客户端能直接把图渲染出来；要存
文件就 decode 逗号后面那段 base64。

```bash
curl http://127.0.0.1:8083/v1/chat/completions \
  -H "Authorization: Bearer sk-gemrelay-..." -H "Content-Type: application/json" \
  -d '{"model":"gemini-image","messages":[{"role":"user","content":"画一只戴宇航员头盔的橘猫"}]}'
```

Gemini 产物链接需要 Cookie 才能下载，因此服务端先下载字节，再转为 base64 返回给客户端。
`completion_tokens` 仅统计附带的说明文字，图片和音频的 base64 数据排除在外。
这两个模型需要登录态，并列入登录模型清单。`size` 当前接受后忽略，`n > 1` 返回 400。

默认单轮路径将 `messages` 拼成一个 prompt；开启 `multi_turn` 后，服务端可复用 Gemini 会话，命中续接时只发最新消息。未命中续接或带附件时仍会重发历史，受单次输入长度限制。实测请求体上限约 **13 万 UTF-8 字节**；本地检查用于提前识别超限，避免上游截掉尾部的最新消息。

登录请求的超长对话会转为 `message.txt` 附件发送，可用长度从约 13 万提高到约 **16 万字节**。
附件限制按模型可见内容的总量计算，超出部分即使上传也可能无法读取。固定总量、仅移动暗号偏移的测试中，
157,833 处可读，163,371 处不可读；拆成多份附件后总额度相同。
更长的对话需要客户端压缩。匿名请求超限时返回 400 `context_length_exceeded`。

<a id="protocol"></a>

## OpenAI 协议覆盖

| 路径 | 状态 | 备注 |
|---|---|---|
| `POST /v1/chat/completions` | 支持 | 增量正文和思考链。带工具时流式转发围栏外正文，完整解析后返回结构化工具调用；成功流以 `finish_reason` 和 `[DONE]` 结束，失败发送错误块 |
| `POST /v1/responses` | 支持 | 增量正文和 Responses 事件序列，思考链事件暂缺；工具调用完整解析后返回。成功以 `response.completed` 结束，失败以 `response.failed` 结束 |
| `GET /v1/models` | 支持 | 匿名返回两个文本模型；有启用账号时列出已注册登录模型，实际使用还需账号具备对应权限 |
| `POST /v1/videos`、`GET /v1/videos/{id}`、`GET /v1/videos/{id}/content` | 支持 | 创建视频任务、查询状态、下载 MP4；需要账号具备视频权限 |
| `POST /mcp` | 支持 | Streamable HTTP 的 `web_search` 工具，使用同一把 API Key |
| `GET /` | ✅ | 健康检查，不鉴权，返回 status/version/models |
| `/v1beta/models/…`（Gemini CLI 原生格式） | ❌ | 未实现，只暴露 OpenAI 形状的接口 |
| `/v1/embeddings`、`/v1/images/*`、`/v1/audio/*` | ❌ | 未实现，返回 404 |
| Function calling | ⚠️ | 通过提示词要求模型输出 ` ```tool_call``` ` 块，再用正则解析。查询私有数据与内部系统类工具较可靠；天气等模型可自行回答的问题可能直接作答，发邮件等有副作用的动作可能被拒绝。Codex 等 agent 客户端已可接入。早期版本 4.11.0 修复长系统提示覆盖工具指令的问题；4.12.0 将工具结果压缩为成功/失败信号，减少将空输出误判为失败后的重试，同一测试任务由 26 轮降至 2–4 轮 |
| `tool_choice` | ⚠️ | `none` 跳过工具定义注入；`required` 加强调用要求，指定函数时仅保留该工具。约束依赖模型遵循提示词，测试中天气、2+2 等问题在 `required` 下仍可能被直接回答 |
| `stream_options.include_usage` | ✅ | 在 `finish_reason` 之后补一个 `choices` 为空的 usage chunk |
| `usage` token 数 | ✅ | tiktoken cl100k_base，与管理面板 requests 表同口径 |
| `n` > 1 | ❌ | 返回 400，上游仅提供一个候选 |
| 采样参数 | ➖ | `temperature` / `top_p` / `max_tokens` / `stop` / `seed` / `presence_penalty` / `frequency_penalty` 接受后忽略，网页协议缺少对应控制项 |
| `response_format` / `logprobs` | ➖ | 未实现，收下即忽略 |
| Vision / 图片输入 | ⚠️ | **挂 cookie 时可用**：`image_url`（chat）和 `input_image`（responses）都认，支持 `data:` URL 和 http(s) 链接，单张 12MB 封顶。匿名态返回 400——匿名**能**把图传上去（`content-push.googleapis.com/upload/` 两步 resumable），但对话里引用被上游拒绝（`BardErrorInfo 1100`） |
| Audio | ❌ | 传 `input_audio` 返回 400。网页端有音乐生成（Lyria 3），需要登录态 |

---

继续阅读：[部署与配置](deployment.md) · [API 与模型](api.md) · [账号与代理](operations.md) · [排障](troubleshooting.md) · [开发](development.md)
