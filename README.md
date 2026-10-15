# gemini-web2api-go

把 Gemini 网页端转成 OpenAI 兼容 API 的 Go 服务，自带账号池、代理池和管理面板。
本仓基于 [zexadev/gemini-web2api-go](https://github.com/zexadev/gemini-web2api-go) 修改，包含目录重构、严格模型校验、账号额度查询及移动端管理界面。

<img src="docs/banner.svg" alt="gemini-web2api-go：Gemini 网页端到 OpenAI 兼容 API" width="100%">

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.27.2%2B-00ADD8.svg)](go.mod)

中文 | [English](docs/en/README.md)

项目代理 Gemini 网页协议，不需要 Google API Key。匿名可用两个文本模型；其他模型和媒体能力取决于 Google 账号权限与网页端额度。

## 功能

**接口**
- OpenAI 兼容：`/v1/chat/completions`、`/v1/models`、`/v1/responses`
- `/v1/videos`：OpenAI(Sora) 形状的异步视频生成——`POST` 建任务、`GET /v1/videos/{id}` 轮询、`GET /v1/videos/{id}/content` 下 MP4（要登录态 Pro 号）
- 普通对话真流式：上游每出一帧就转发增量
- Bearer token / `x-api-key` 鉴权，key 可在面板轮换
- `usage` 用 tiktoken 算，`reasoning_tokens` 单列不计入 `completion_tokens`

**模型**
- `gemini-3.6-flash`、`gemini-3.5-flash-lite` 匿名可用，含联网搜索
- `gemini-3.1-pro` 需登录账号有相应权限；上游提供时返回思考链（`reasoning_content`）
- 文本模型提供扩展思考档位，挂 cookie 后可用
- 响应里记录服务端**实际**用了哪个模型，被静默降级一眼可见

**账号与出口**
- 直连使用可配置的浏览器 TLS 指纹，默认 Chrome 146；代理连接使用 Go 标准库客户端
- 每个出口 IP 独立限流：并发 / RPM / RPH 三档
- 代理池：运行时增删改、失败熔断、轮转调度，每个代理是独立限流槽
- Cookie 池：多个 Google 账号按最久未用优先轮转，自动续期 + 保活，每个账号粘住自己的出口

**运维**
- 单二进制，交叉编译 6 平台；容器镜像基于 distroless
- SQLite 持久化：30 天请求明细 + 永久聚合统计（可选 MySQL / PostgreSQL，设 `SQL_DSN` 即可）
- 中文管理面板：概览 / 请求记录 / 代理池 / Cookie 池 / 设置，可在面板调整配置，超时更新已有客户端需重启
- **prompt 和回复内容永不入库**，只存元数据（长度、耗时、模型、状态）

## 快速开始

源码构建需要 Go 1.27.2 或更新版本。在本仓根目录执行：

```bash
go build -o gemini-web2api-go ./cmd/gemini-web2api
./gemini-web2api-go --port 8083 --admin-token your-admin-token
```

打开 `http://localhost:8083/admin`，用管理员令牌登录，在「设置」页查看 API Key。默认数据库为 `./data/gemini.db`，首次启动会生成 API Key。

调用示例：

```bash
curl http://localhost:8083/v1/chat/completions \
  -H "Authorization: Bearer sk-gemini-..." \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-3.6-flash",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

客户端 Base URL 填 `http://localhost:8083/v1`，API Key 使用面板中的值，模型可先选 `gemini-3.6-flash`。

Docker、数据库和配置项见[部署与配置](docs/deployment.md)。Compose 默认构建本仓源码；预编译二进制与远程镜像链接指向原项目，不保证包含本仓新增功能。

## 文档

| 需要做什么 | 文档 |
|---|---|
| 部署服务、选择数据库、设置参数 | [部署与配置](docs/deployment.md) |
| 接入客户端、选择模型、使用 MCP、查看协议支持 | [API、模型与客户端](docs/api.md) |
| 导入 Cookie、查询额度、管理代理和限流 | [管理面板、账号与代理](docs/operations.md) |
| 查询错误原因、能力限制与常见问题 | [限制与故障排查](docs/troubleshooting.md) |
| 理解目录、构建与测试 | [开发与项目结构](docs/development.md) · [架构约束](docs/architecture.md) |

## 使用前注意

- 本项目依赖逆向的网页协议，上游改动可能影响可用性，不等同于 Google 官方 API。
- Function calling 采用提示词约定；部分 OpenAI 参数没有对应能力，详见[协议覆盖](docs/api.md#protocol)。
- 严格模型校验默认关闭。开启后会把已确认的模型不匹配作为错误；实时流式已发出的内容无法撤回。
- 账号权限、上下文长度与出口限流仍受网页端限制；敏感操作不要仅依赖模型的成功描述。
- 服务只存请求元数据，不把 prompt 和回复正文写入数据库。部署时请设置管理员令牌并限制访问范围。

## 致谢

- [zexadev/gemini-web2api-go](https://github.com/zexadev/gemini-web2api-go)：本仓基于该 Go 项目修改，感谢原作者的实现与协议研究。
- [bogdanfinn/tls-client](https://github.com/bogdanfinn/tls-client) — Chrome 真指纹 TLS 库
- [pkoukk/tiktoken-go](https://github.com/pkoukk/tiktoken-go) — BPE tokenizer
- [modernc.org/sqlite](https://gitlab.com/cznic/sqlite) — 纯 Go SQLite（CGO-free，alpine 直接编）

## License

MIT — 详见 [LICENSE](LICENSE)

## 友情链接

- [Sophomoresty/gemini-web2api](https://github.com/Sophomoresty/gemini-web2api) —— 同类的 Python 实现，早期摸 Gemini 网页协议时借鉴过它的思路
