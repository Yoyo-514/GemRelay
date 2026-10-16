# 部署与配置

[首页](../README.md) · [English](en/deployment.md)

可使用已发布的 Docker 镜像或二进制部署。以下镜像示例使用 `latest` 标签，将 `your-admin-token` 换成自己的管理员令牌。

<a id="quick-start"></a>

## 快速开始

### Docker 镜像

镜像发布在 [GHCR](https://github.com/Yoyo-514/GemRelay/pkgs/container/gemrelay)，支持 Linux amd64 / arm64。在 Linux 部署机器执行：

```bash
docker pull ghcr.io/yoyo-514/gemrelay:latest
docker run -d --name gemrelay \
  -p 0.0.0.0:8083:8083 \
  -v "gemrelay-data:/data" \
  ghcr.io/yoyo-514/gemrelay:latest \
  --db "/data/gemrelay.db" --port 8083 --admin-token your-admin-token
```

容器内的数据库路径为 `/data/gemrelay.db`，由 `gemrelay-data` 卷持久化。升级前请备份数据。端口绑定部署机器的 `0.0.0.0`，可通过公网或局域网 IP 访问。

版本标签（`vX.Y.Z`）对应正式发布；`main` 和 `latest` 跟随主分支构建。按版本部署时，使用发布页对应的镜像标签。

### Docker Compose

Linux 和 Windows Docker Desktop 均可使用 Docker Compose v2。在部署目录保存以下内容为 `compose.yaml`：

```yaml
services:
  gemrelay:
    image: ghcr.io/yoyo-514/gemrelay:latest
    container_name: gemrelay
    restart: unless-stopped
    ports:
      - "0.0.0.0:8083:8083"
    volumes:
      - gemrelay-data:/data
    command: ["--db", "/data/gemrelay.db", "--port", "8083", "--admin-token", "your-admin-token"]

volumes:
  gemrelay-data:
```

在同一目录执行：

```bash
docker compose -f "compose.yaml" pull
docker compose -f "compose.yaml" up -d
```

### 二进制

从[最新 Release](https://github.com/Yoyo-514/GemRelay/releases/latest) 下载对应系统和架构的单文件程序。发布文件名带有版本号，可按下列后缀选择：

| 系统 | amd64（x86-64） | arm64 |
|---|---|---|
| Linux | `*_linux_amd64` | `*_linux_arm64` |
| macOS | `*_darwin_amd64` | `*_darwin_arm64` |
| Windows | `*_windows_amd64.exe` | `*_windows_arm64.exe` |

下载后，将 Linux / macOS 程序重命名为 `gemrelay`，Windows 程序重命名为 `gemrelay.exe`。

Linux / macOS 在程序所在目录添加执行权限并启动：

```bash
chmod +x "./gemrelay"
"./gemrelay" --port 8083 --admin-token your-admin-token
```

Windows 在程序所在目录用 PowerShell 启动：

```powershell
& "./gemrelay.exe" --port 8083 --admin-token your-admin-token
```

二进制将数据保存在工作目录的 `./data/gemrelay.db`，可通过 `--db` 指定其他路径。

### 登录与接入

服务默认监听 `0.0.0.0:8083`。将 `SERVER_IP` 换成服务器的公网或局域网 IP，并在主机防火墙和云服务器安全组中放行 TCP 8083 端口。

打开 `http://SERVER_IP:8083/admin`，用管理员令牌登录。首次启动会生成 API Key，启动输出显示脱敏值，完整值在「设置」页查看。

客户端 Base URL 填 `http://SERVER_IP:8083/v1`，API Key 使用面板中的值。调用示例见 [API、模型与客户端](api.md)。

## 数据库

数据库默认为 SQLite。使用 MySQL / PostgreSQL 时，设置 `SQL_DSN` 环境变量：

```bash
# MySQL（也接受 go-sql-driver 原生 user:pass@tcp(host:3306)/dbname）
SQL_DSN="mysql://user:pass@host:3306/dbname"
# PostgreSQL
SQL_DSN="postgres://user:pass@host:5432/dbname?sslmode=disable"
```

建表自动完成，三种库共用同一套 schema。单实例可使用默认 SQLite，多实例共享池数据可使用 MySQL / PostgreSQL。

## 从源码构建

### Go 二进制

需要 Go 1.27.2 或更新版本。在仓库根目录执行：

```bash
go build -o gemrelay ./cmd/gemrelay
./gemrelay --port 8083 --admin-token your-admin-token
```

### 本地 Docker 镜像

在仓库根目录构建并运行：

```bash
docker build -f build/package/Dockerfile -t gemrelay:local .
docker run -d --name gemrelay \
  -p 0.0.0.0:8083:8083 \
  -v gemrelay-data:/data \
  -e ADMIN_TOKEN=your-admin-token \
  gemrelay:local
```

仓库中的 [Compose 配置](../deployments/docker-compose.yml) 默认拉取远程镜像。选择本地构建时，将 `image` 改为 `gemrelay:local`，`pull_policy` 改为 `build`，并启用注释中的 `build` 块。构建上下文为仓库根目录，镜像定义位于 [build/package/Dockerfile](../build/package/Dockerfile)。[构建忽略规则](../.dockerignore)采用允许列表，包含生产源码、模块清单与嵌入资源。

启用本地构建后，在 Git Bash 或 Linux 终端中提供管理员令牌并启动：

```bash
ADMIN_TOKEN=your-admin-token docker compose -f deployments/docker-compose.yml up -d --build
```

PowerShell 可先设置 `ADMIN_TOKEN`，再执行 Compose 的 `up -d --build`。也可通过 `docker compose --env-file ".env" -f "deployments/docker-compose.yml" up -d --build` 读取本地环境文件；含令牌或数据库凭据的文件请留在本地。

仓库 Compose 的 healthcheck 检查二进制能否运行；HTTP 就绪检查使用 `GET /`。

<a id="configuration"></a>

## 配置

配置分为运行时配置和部署期配置，凭证在管理面板中维护。

### 运行时配置 → 管理面板「设置」页

保存后发布运行时配置并写入数据库，优先级高于 `config.json` 和命令行参数。已有网络客户端保留创建时的超时，修改超时后重启服务可确保全部出口更新。

| 项 | 说明 |
|---|---|
| 默认模型 | 客户端没传 `model` 时用哪个 |
| 每 slot 并发 / RPM / RPH | 限流额度，0 = 不限 |
| Prompt 字节上限 | 按 UTF-8 字节计算，默认 128000，0 = 关闭本地检查。超限时，登录请求转成文本附件发送（可用长度约 16 万字节），匿名请求返回 400 `context_length_exceeded`；原文保持完整 |
| 多轮（`multi_turn`） | 默认关。通过 Gemini 原生 conversation_id 续接：客户端每轮发送全历史，服务端识别续接后只向上游发送最新消息，历史保留在上游。登录态和匿名态均可用，可减少单次请求字节数；模型上下文窗口保持原有大小，超窗的早期内容仍会被挤出，超长文档仍受输入长度限制 |
| 自动删会话（`auto_delete_conversation`） | 默认关。仅用于登录态单轮请求，成功后异步尝试删除网页会话；删除失败时保留已生成的响应 |
| 临时对话（`temporary_chat`） | 默认关。使用网页端原生「临时对话」，会话留在临时模式，排除在账号会话列表与模型训练之外，省去后续删除请求。仅登录态单轮生效；多轮续接使用持久会话 |
| 匿名优先（`anon_first`） | 默认关。允许匿名的模型在请求仅含文本时优先匿名执行，附带工具定义也适用；登录模型、思考档、媒体工具和附件使用账号 |
| 按额度选号（`quota_aware_pick`） | 默认关闭。按剩余额度分带选择账号，同带沿用健康度和轮转顺序；读取 20 秒内的快照，按需异步补查。详见[按额度选号](operations.md#按额度选号) |
| 额度轮询间隔（`quota_poll_interval_sec`） | 默认 60 秒，范围 10–3600 秒。Cookie 池页面可见时逐轮刷新，每轮最多三路并发 |
| 重试次数 / 重试间隔 / 上游超时 | |
| 严格模型校验（`strict_model`） | 默认关。确认响应帧自报的模型与请求不符时结束请求：未开流返回 `429 insufficient_quota`、`code: model_downgraded`；Chat 已开流发送同分类错误，Responses 发送 `response.failed`，均以失败结束。实时输出照常转发，客户端可能已收到部分正文。校验覆盖有模型证据的已知文本模型，别名和思考档按基础模型判断，思考级别本身在校验范围之外。其余情况放行。模型不匹配直接报错，换号重试由调用方决定；原因可能是额度耗尽或账号尚未开放该模型 |
| 明细保留天数 | 过期只删明细，聚合数据永久保留 |
| TLS 指纹 | `chrome_146`（默认）/ `chrome_144` / `chrome_133` / `firefox_147` / `safari_16_0` / `safari_ios_17_0` |
| Gemini `bl` 版本 | 上游前端版本号，过期时改这里 |
| 打印请求日志 | |

后端校验所有配置值的范围，例如 `retry_attempts` 为 1–10、超时为 5–600 秒，
超出范围时返回原因。

### 凭证 → 也在面板

| 项 | 说明 |
|---|---|
| API Key | 首次启动自动生成，面板里可轮换或自定义 |
| Google Cookie | 在「Cookie 池」导入。有启用账号时列出登录模型；实际权限与额度仍由上游账号决定 |

两者均保存在数据库中。已鉴权的「设置」页可以查看和复制 API Key；Cookie 列表仅返回脱敏摘要。

### 启动配置

以下项目在修改后重启进程生效。Docker 部署还需在 Compose 或 `docker run` 中设置端口映射和数据卷：

| 项 | 位置 |
|---|---|
| 监听地址 | `config.json` 中的 `host`，默认 `0.0.0.0` |
| 监听端口 | `--port`；容器端口通过 `ports` 或 `-p` 映射 |
| 数据库路径 | `--db`；容器数据通过 `volumes` 或 `-v` 持久化 |
| 管理员令牌 | `--admin-token` 或 `ADMIN_TOKEN`，用于面板登录 |

`API_KEY` 环境变量将 API Key 固定为部署值，面板显示为只读。

`--proxy` 和 `--cookie-file` 用于启动导入：将值写入代理池 /
Cookie 池，分别按 URL 和 Cookie 内容去重，后续从面板管理。修改参数后重启即可导入新值。

启动参数和 `config.json` 用于指定部署参数与运行时初值。配置优先级：
**面板改动 > CLI flag / `config.json` > 内置默认**。

直接运行二进制时，可以把 [`configs/config.example.json`](../configs/config.example.json) 复制成 `config.json` 当启动模板
（配置读取范围为显式路径和工作目录的 `config.json`；缺失时使用内置默认值）。里面的调优项面板改了就以面板为准，
`config.json` 只决定第一次启动的初值。

命令行参数全集：

| flag | 说明 |
|---|---|
| `--port` | 监听端口，默认 8083 |
| `--config` | 指定 `config.json` 路径 |
| `--db` | SQLite 路径，默认 `./data/gemrelay.db` |
| `--admin-token` | 面板登录 token；公网或局域网部署时设置。本机回环监听的调试可留空以关闭面板鉴权 |
| `--api-key` | 将 `/v1/*` 的 key 固定为部署值，面板只读，等价于 `API_KEY` 环境变量 |
| `--cookie-file` | 启动时把文件里的 cookie 导入 Cookie 池 |
| `--proxy` | 启动时把这个代理导入代理池 |
| `--impersonate` | TLS 指纹档位 |
| `--version` | 打印版本退出（Docker healthcheck 用的就是它） |

---

继续阅读：[部署与配置](deployment.md) · [API 与模型](api.md) · [账号与代理](operations.md) · [排障](troubleshooting.md) · [开发](development.md)
