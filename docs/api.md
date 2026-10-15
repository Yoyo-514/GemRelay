# API、模型与客户端

[首页](../README.md) · [English](en/api.md)

命令均从仓库根目录执行。

## 这是什么

把这种调用：
```
[OpenAI SDK / Cherry Studio / Cursor / dify / newapi / ...]
    ↓ http://localhost:8083/v1/chat/completions
[GemRelay]
    ↓ 逆向 gemini.google.com 网页协议
[Google Gemini 网页端]
```

本项目直接代理网页协议，不封装 Google 官方 API（[generativelanguage.googleapis.com](https://generativelanguage.googleapis.com)）。无需 Google API Key 或官方 API 计费配额；网页端账号权限和用量限制仍然适用。

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

**Gemini CLI 接不了**：它要的是 Google 原生的 `/v1beta/models/{model}:generateContent`，本项目只暴露
OpenAI 形状的接口，没做 `/v1beta`。

另有一个不鉴权的健康检查 `GET /`，返回 `{"status":"ok","version":…,"models":[…]}`，给探活用。

<a id="mcp"></a>

## MCP（web_search 工具）

除了 OpenAI 接口，同一个进程、同一个端口还挂了一个 **MCP server**，在 `/mcp` 上，
把 Gemini 网页端的**联网搜索**暴露成一个 `web_search` 工具。让 Claude Desktop /
Claude Code / Cursor 这类 MCP 客户端能「用 Gemini 去搜网」，返回**合成答案 + 来源链接**。

不用单独起进程、不用额外部署——起了后端就有。传输是 HTTP（Streamable HTTP），
所以远程客户端连 URL 就能用，复用后端的账号池 / 代理池 / 限流。匿名即可搜，不必挂 cookie。

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
| `gemini-3.1-pro` | 最强，**要配 cookie**；每次回答都带思考链 |
| `gemini-3.8-flash` | 新款 Flash，**要付费 Gemini 账号的 cookie**（免费号降级成 3.5 Flash-Lite）。`gemini-3.7-flash` 是它的别名——Google 把 3.7 那个 hex 原地升成了 3.8 |
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

没配 cookie 时 `/v1/models` 只返回前两个，选 `gemini-3.1-pro` 会直接报错并说明
原因。因为匿名请求它必然被静默降级成 3.5 Flash-Lite——与其让客户端拿到一个
"成功但其实不是 Pro"的回复，不如在选型时就失败。

原项目曾用有效 Cookie 连续调用 6 次，服务端均回报 `3.1 Pro`。账号权限与额度可能变化，当前结果以请求明细中的实际模型为准，也可开启严格模型校验。

`-thinking` 对应网页 UI 的扩展思考开关，与基础模型标识独立。Argon 使用低 / 中 / 高档位。
部分显示名带 `Extended`（如 `3.6 Flash Extended`），但显示名不能证明思考级别。原项目的历史输出长度记录为 2467 / 1059 / 583 字符，普通版为 0 / 0 / 268；这些长度不构成输出保证。
扩展思考只在登录态生效，匿名模型列表不暴露这些档位。

仅暴露当前注册的模型与别名。旧的 `gemini-3.5-flash`、`gemini-3.5-flash-thinking`、
`gemini-3.5-flash-thinking-lite`、`gemini-auto`、`gemini-flash-lite` **已移除**
（传了会返回 400）——它们在服务端没有对应条目，留着只会让人以为有五种不同
的模型可选。

> **`@think=N` 已废弃。** 抓包确认 `inner[17]` 是会话轮次索引，不是思考深度。单轮路径写 0，服务端多轮续接路径递增。后缀仍接受但忽略；设置思考级别请使用已注册的思考版模型或 Argon 档位。

<a id="reasoning"></a>

### 思考链（reasoning_content）

上游提供思考链时，Chat Completions 通过 `reasoning_content` 暴露该内容。是否返回取决于模型设置、账号权限和上游结果，没有内容时省略该字段。Responses 路径目前不发送思考链事件。

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

流式按 `delta.reasoning_content` 推，且**思考块全部推完才开始推正文**（上游就是这个
顺序），客户端观感跟网页端一致。

`reasoning_tokens` **单独计数、不含在 `completion_tokens` 里**：思考链默认被客户端
折叠不展示，算进去等于让用户为看不见的输出买单（下游 newapi 按 `completion_tokens`
计费）。要计费的自己把两者相加。

### 已知的能力边界

匿名调用（不挂 cookie）只能拿到上面两个文本模型 + Gemini 自带的联网搜索。
`gemini-3.1-pro` 匿名时被静默降级成 3.5 Flash-Lite，所以干脆不暴露。

挂上 cookie 额外解锁：`gemini-3.1-pro`、**账号有权使用的扩展思考版与 Argon**、**读图 / 读视频**、
**更长的上下文**（超长对话自动转成文本附件发，见下文），以及 **生图（`gemini-image`）**、
**音乐（`gemini-music`）**、**画布（`gemini-canvas`）**、**生视频（`gemini-video`，要 Pro 号）**。

深度研究仍**未实现**（多步异步流程）。管理面板的「实际模型」列会把服务端实际用了哪个
模型标出来，降级一眼可见。

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

不转外链是有意的——Gemini 的产物链要带 cookie 才下得到，直接把链给客户端它打不开，
所以服务端下回字节再转 base64。产物的 base64 **不计进 `completion_tokens`**（否则一张图
上百万 token，下游按它计费就离谱了），只算模型附带的说明文字。都要登录态，没 cookie
时这俩模型不进 `/v1/models`。`size` 没有对应旋钮，当前会忽略；`n > 1` 会返回 400。

默认单轮路径将 `messages` 拼成一个 prompt；开启 `multi_turn` 后，服务端可复用 Gemini 会话，命中续接时只发最新消息。未命中续接或带附件时仍会重发历史，因此可能撞上单次输入长度限制。原项目测得请求体约 **13 万 UTF-8 字节**；若不做本地检查，上游可能从尾部截断，丢掉最新消息。

挂 cookie 时超长对话会自动转成 `message.txt` 附件发上去，绕开请求体那堵墙；**但附件
另有一堵墙**：模型能看到的内容合计约 **16 万字节**，超出部分传上去了也读不到（实测总量
固定、只挪暗号偏移：157,833 处读得到、163,371 处读不到；切成多份附件不涨额度）。
所以挂 cookie 把可用长度从 13 万提到约 16 万，**真正的长对话仍需客户端自己压缩**。
没挂 cookie 时直接返回 400 `context_length_exceeded`，不静默丢数据。

<a id="protocol"></a>

## OpenAI 协议覆盖

| 路径 | 状态 | 备注 |
|---|---|---|
| `POST /v1/chat/completions` | 支持 | 增量正文和思考链。带工具时流式转发围栏外正文，完整解析后返回结构化工具调用；成功流以 `finish_reason` 和 `[DONE]` 结束，失败发送错误块 |
| `POST /v1/responses` | 支持 | 增量正文和 Responses 事件序列；工具调用完整解析后返回，目前不发送思考链事件。失败以 `response.failed` 结束，不发送 `response.completed` |
| `GET /v1/models` | 支持 | 匿名返回两个文本模型；有启用账号时列出已注册登录模型，不代表上游授予该账号全部模型权限 |
| `POST /v1/videos`、`GET /v1/videos/{id}`、`GET /v1/videos/{id}/content` | 支持 | 创建视频任务、查询状态、下载 MP4；需要账号具备视频权限 |
| `POST /mcp` | 支持 | Streamable HTTP 的 `web_search` 工具，使用同一把 API Key |
| `GET /` | ✅ | 健康检查，不鉴权，返回 status/version/models |
| `/v1beta/models/…`（Gemini CLI 原生格式） | ❌ | 未实现，只暴露 OpenAI 形状的接口 |
| `/v1/embeddings`、`/v1/images/*`、`/v1/audio/*` | ❌ | 未实现，返回 404 |
| Function calling | ⚠️ | Prompt 级实现（让模型输出 ` ```tool_call``` ` 块再 regex 解析），不是真协议层。**查私有数据/内部系统类可靠**，但 Gemini 自己能做的（如查天气）会被它直接回答，有副作用的动作（如发邮件）会被拒绝。**agentic 客户端（Codex 等）已可用**：4.11.0 前它们几十 KB 的系统提示会把工具指令冲没导致「已读乱回」，现已修复；4.12.0 起工具结果压成清爽成功/失败信号，弱模型不再因「命令无输出」误判失败而反复重试（实测同一任务从 26 轮循环降到 2-4 轮收尾）|
| `tool_choice` | ⚠️ | `none` 完全不注入工具定义；`required` 和指定函数会加强制措辞、并把其余工具从 prompt 裁掉。但 prompt 级实现**无法真正强制**——实测模型自己答得上来的问题（天气、2+2）即使 `required` 也照样直接作答 |
| `stream_options.include_usage` | ✅ | 在 `finish_reason` 之后补一个 `choices` 为空的 usage chunk |
| `usage` token 数 | ✅ | tiktoken cl100k_base，与管理面板 requests 表同口径 |
| `n` > 1 | ❌ | 返回 400。上游只给一个候选，静默按 1 处理会让客户端少拿结果 |
| 采样参数 | ➖ | `temperature` / `top_p` / `max_tokens` / `stop` / `seed` / `presence_penalty` / `frequency_penalty` **收下即忽略，不报错**。Gemini 网页协议没有这些旋钮 |
| `response_format` / `logprobs` | ➖ | 未实现，收下即忽略 |
| Vision / 图片输入 | ⚠️ | **挂 cookie 时可用**：`image_url`（chat）和 `input_image`（responses）都认，支持 `data:` URL 和 http(s) 链接，单张 12MB 封顶。匿名态返回 400——匿名**能**把图传上去（`content-push.googleapis.com/upload/` 两步 resumable），但对话里引用被上游拒绝（`BardErrorInfo 1100`） |
| Audio | ❌ | 传 `input_audio` 返回 400。网页端有音乐生成（Lyria 3），需要登录态 |

---

继续阅读：[部署与配置](deployment.md) · [API 与模型](api.md) · [账号与代理](operations.md) · [排障](troubleshooting.md) · [开发](development.md)
