# 部署与配置

[首页](../README.md) · [English](en/deployment.md)

命令均从仓库根目录执行。

> 本文中的 Release 与远程镜像地址指向[原项目](https://github.com/zexadev/gemini-web2api-go)，不保证包含本仓新增功能。本仓的 Compose 默认构建当前源码，不拉取原项目镜像。

<a id="quick-start"></a>

## 快速开始

### 下载二进制（最省事）

[Releases](https://github.com/zexadev/gemini-web2api-go/releases) 里挑对应平台的下载，
不用 Go、不用 Docker，单个文件就是全部：

```bash
chmod +x gemini-web2api-go_*
./gemini-web2api-go_* --port 8083 --admin-token your-admin-token
```

数据默认落在 `./data/gemrelay.db`，换位置加 `--db /your/path.db`。

想用 MySQL / PostgreSQL，设 `SQL_DSN` 环境变量即可（不设就是 SQLite，无需改动）：

```bash
# MySQL（也接受 go-sql-driver 原生 user:pass@tcp(host:3306)/dbname）
SQL_DSN="mysql://user:pass@host:3306/dbname"
# PostgreSQL
SQL_DSN="postgres://user:pass@host:5432/dbname?sslmode=disable"
```

建表自动完成，三种库共用同一套 schema。单机就用默认 SQLite 最省事，多实例共享一个池子才需要 MySQL/PG。

### Docker（不用源码）

```bash
docker run -d --name gemini-web2api \
  -p 127.0.0.1:8083:8083 \
  -v "$PWD/data:/data" \
  -e ADMIN_TOKEN=your-admin-token \
  ghcr.io/zexadev/gemini-web2api-go:latest
```

使用 Docker Compose v2 时，从当前仓库根目录执行。配置会构建本地镜像 `gemrelay:local`，管理员令牌必须提供：

```bash
ADMIN_TOKEN=your-admin-token docker compose -f deployments/docker-compose.yml up -d --build
```

### 从源码跑

需要 Go 1.27.2 或更新版本。在本仓根目录执行，不要用原项目源码代替当前版本：

```bash
go build -o gemrelay ./cmd/gemrelay
./gemrelay --port 8083 --admin-token your-admin-token
```

本仓的 [部署配置](../deployments/docker-compose.yml) 已启用源码构建，需要完整仓库，不能只下载该文件运行。构建上下文为仓库根目录，镜像定义位于 [build/package/Dockerfile](../build/package/Dockerfile)。[构建忽略规则](../.dockerignore)仅允许生产源码、模块清单与嵌入资源进入上下文。

PowerShell 可先设置环境变量，再运行同一条 Compose 命令。也可通过 `docker compose --env-file .env -f deployments/docker-compose.yml up -d --build` 读取本地环境文件；不要提交含令牌或数据库凭据的文件。

GemRelay 默认使用独立的 `gemrelay-data` 卷；请在更换部署前自行备份。当前 healthcheck 只确认二进制能运行；HTTP 就绪检查请请求 `GET /`。

也可从仓库根目录直接构建镜像：`docker build -f build/package/Dockerfile -t gemrelay .`。

旧版本启动输出示例（版本、模型和状态以当前运行结果为准）：

```
gemini-web2api-go v4.0.0
  Listening:   http://0.0.0.0:8083
  Base URL:    http://localhost:8083/v1
  API key:     sk-gemini-XX...XXXX  (mutable in admin UI)
  Admin UI:    http://localhost:8083/admin  (token auth)
  DB:          ./data/gemini.db
  Models:      [gemini-3.5-flash-lite gemini-3.6-flash]
  Cookie:      none (anonymous)
  Proxy:       none
  Impersonate: chrome_146
  Tokenizer:   tiktoken cl100k_base
  Per-IP 限流: 并发=5 / RPM=30 / RPH=80
  Retry:       3x / 2s
```

首次启动会自动生成 API key（banner 里是打码的，完整值在管理面板「设置」页）。

<a id="configuration"></a>

## 配置

配置只有两个地方，按"改了要不要重启"分：

### 运行时配置 → 管理面板「设置」页

保存后发布运行时配置并写入数据库，优先级高于 `config.json` 和命令行参数。已有网络客户端保留创建时的超时，修改超时后重启服务可确保全部出口更新。

| 项 | 说明 |
|---|---|
| 默认模型 | 客户端没传 `model` 时用哪个 |
| 每 slot 并发 / RPM / RPH | 限流额度，0 = 不限 |
| Prompt 字节上限 | 超了：挂 cookie 时转成文本附件发（可用长度到约 16 万字节），没挂时返回 400 `context_length_exceeded`。都不静默截断。按 UTF-8 字节算（上游的墙跟 token 无关），默认 128000。0 = 不限 |
| 多轮（`multi_turn`） | 默认关。开启后走 Gemini 原生 conversation_id 服务端续接——客户端每轮重发全历史，服务端识别续接后只发最新一句、历史留服务端，长会话不再撞单请求字节墙。登录/匿名都可用。**注意**：不放大模型上下文窗口，超窗的早期内容仍会被挤出，解决的是"长对话不撞墙 + 保住最近上下文"，不是"喂超长文档"。 |
| 自动删会话（`auto_delete_conversation`） | 默认关。登录态单轮请求成功后异步尝试删除网页会话，删除失败不影响响应；服务端多轮路径不执行自动删除 |
| 临时对话（`temporary_chat`） | 默认关。开启后走 Gemini 网页端原生「临时对话」：会话压根不进账号的会话列表、不用于模型训练，比「自动删会话」更干净，还省掉那次删除请求。只登录态有意义；开了「服务端多轮续接」时自动不生效（临时对话不留列表，多轮要靠服务端留会话）。 |
| 匿名优先（`anon_first`） | 默认关。开启后，模型不要求登录且没有附件的请求可以匿名执行；工具定义本身不强制登录。登录模型、思考档、媒体工具和附件仍需要账号 |
| 重试次数 / 重试间隔 / 上游超时 | |
| 严格模型校验（`strict_model`） | 默认关。开启后，响应帧中自报的模型与请求不符时明确失败：未开流返回 `429 insufficient_quota`、`code: model_downgraded`；Chat 已开流发送同分类错误后结束，Responses 发送 `response.failed`，不再发送成功终态。保留实时流式，报错前可能已经发出部分正文，无法撤回。没有自报模型、未知模型或媒体工具不参与判断。别名和思考档共用基础模型判据，不保证识别思考级别被降低。不会自动换号重试；不匹配也可能是账号尚未开放该模型，不一定是额度耗尽。 |
| 明细保留天数 | 过期只删明细，聚合数据永久保留 |
| TLS 指纹 | `chrome_146`（默认）/ `chrome_144` / `chrome_133` / `firefox_147` / `safari_16_0` / `safari_ios_17_0` |
| Gemini `bl` 版本 | 上游前端版本号，过期时改这里 |
| 打印请求日志 | |

所有值都在后端做范围校验（比如 `retry_attempts` 只接受 1-10、超时 5-600 秒），
非法值会被拒绝并说明原因——浏览器端的限制随手就能绕过，真正的关卡在服务端。

### 凭证 → 也在面板

| 项 | 说明 |
|---|---|
| API Key | 首次启动自动生成，面板里可轮换或自定义 |
| Google Cookie | 在「Cookie 池」导入。有启用账号时列出登录模型；实际权限与额度仍由上游账号决定 |

两者都存在数据库里。已鉴权的「设置」页可以查看和复制 API Key；Cookie 列表只返回脱敏摘要，不回传已保存的完整 Cookie。

### 部署期配置 → [部署配置](../deployments/docker-compose.yml)

只剩改了必须重启进程的：

| 项 | 位置 |
|---|---|
| 监听端口 | `ports` + `command: --port` |
| 数据库路径 | `volumes` + `command: --db` |
| `ADMIN_TOKEN` | `environment`，面板登录 token |

`API_KEY` 环境变量会锁死 API key（面板改不了），用于不希望运行时被改的部署。

`--proxy` 和 `--cookie-file` 是**播种参数**，不是第二套配置：启动时把值导入代理池 /
Cookie 池（按 URL、cookie 内容去重），之后一律从面板管理。改了重启即生效。

命令行参数仍然可用，定位是本地调试时的临时覆盖。优先级：
**面板改动 > CLI flag / `config.json` > 内置默认**。

不用 Docker 直接跑二进制时，可以把 [`configs/config.example.json`](../configs/config.example.json) 复制成 `config.json` 当启动模板
（配置读取范围为显式路径和工作目录的 `config.json`；缺失时使用内置默认值）。里面的调优项面板改了就以面板为准，
`config.json` 只决定第一次启动的初值。

命令行参数全集：

| flag | 说明 |
|---|---|
| `--port` | 监听端口，默认 8083 |
| `--config` | 指定 `config.json` 路径 |
| `--db` | SQLite 路径，默认 `./data/gemrelay.db` |
| `--admin-token` | 面板登录 token，留空 = 面板不鉴权（只有绑 127.0.0.1 才可接受） |
| `--api-key` | 锁定 `/v1/*` 的 key（面板改不了），等价于 `API_KEY` 环境变量 |
| `--cookie-file` | 启动时把文件里的 cookie 导入 Cookie 池 |
| `--proxy` | 启动时把这个代理导入代理池 |
| `--impersonate` | TLS 指纹档位 |
| `--version` | 打印版本退出（Docker healthcheck 用的就是它） |

---

继续阅读：[部署与配置](deployment.md) · [API 与模型](api.md) · [账号与代理](operations.md) · [排障](troubleshooting.md) · [开发](development.md)
