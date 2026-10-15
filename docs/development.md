# 开发与项目结构

[首页](../README.md) · [English](en/development.md)

命令均从仓库根目录执行。

## 项目结构

源码构建要求 Go 1.27.2 或更新版本；容器构建使用相同版本，发布流水线读取模块清单中的版本要求。

布局参考 [golang-standards/project-layout](https://github.com/golang-standards/project-layout/blob/master/README_zh-CN.md)，按实际需要采用目录；全部测试集中在 [tests/](../tests/)。

| 路径 | 职责 |
|---|---|
| [cmd/gemrelay/](../cmd/gemrelay/) | 程序入口 |
| [internal/app/](../internal/app/) | 启动装配与后台任务调度 |
| [internal/config/](../internal/config/) | 启动配置、运行时快照和版本 |
| [internal/httpapi/](../internal/httpapi/) | 路由、鉴权、OpenAI/MCP 协议适配、管理端和 SSE |
| [internal/gateway/](../internal/gateway/) | 生成编排、多轮、视频任务、消息与工具转换 |
| [internal/gemini/](../internal/gemini/) | Gemini 网页协议、上传、响应解析、媒体与刷新协议 |
| [internal/pool/](../internal/pool/) | 账号与代理池、出口绑定、容量和健康状态 |
| [internal/cookies/](../internal/cookies/) | Cookie 归一化、提取、身份判定与刷新合并 |
| [internal/storage/](../internal/storage/) | 数据库连接、方言、事务、统计和持久化 |
| [internal/transport/](../internal/transport/) | TLS 指纹、浏览器请求头和网络客户端 |
| [tests/](../tests/) | 按职责分组的外部测试、探针、隔离辅助与数据 |
| [web/admin/](../web/admin/) | 管理页面、Chart.js 和资源嵌入声明，随二进制发布 |
| [configs/](../configs/) | 配置模板，只从显式路径或工作目录读取 |
| [build/package/](../build/package/) | 容器镜像构建定义 |
| [deployments/](../deployments/) | Compose 部署配置 |
| [.github/workflows/](../.github/workflows/) | 镜像与二进制发布流水线 |

在仓库根目录执行验证：

Windows 若需要带扩展名的可执行文件，可使用 `go build -o gemrelay.exe ./cmd/gemrelay`；下方的全包构建检查不会在根目录留下可执行文件。

```bash
go build ./...
go test ./... -count=1
go vet ./...
```

测试按包独立使用临时 SQLite，不继承生产数据库连接串；真实上游上传探针仅在显式设置探针环境变量后运行。包职责、依赖约束和迁移说明见 [架构说明](architecture.md)。

GemRelay 按独立项目部署，默认使用自己的配置目录、数据库文件和数据卷。

## 持续集成

PR 执行 Go 格式检查、`go vet`、测试和构建。镜像及二进制发布复用同一份检查，版本标签还需与代码版本一致。
主分支推送继续发布镜像；版本标签继续发布镜像和 Release。在非版本标签引用上手动触发 Release 工作流时，构建产物作为 Actions artifact 保留 7 天，不创建正式 Release。

## Windows 上使用 WSLC 验证容器

本机安装 WSLC 时，可以直接构建镜像，不必依赖普通 WSL 发行版：

```powershell
wslc build --progress plain -f "build/package/Dockerfile" -t "gemrelay:local" "."
wslc run --rm --publish "127.0.0.1:18083:8083" --env "ADMIN_TOKEN=local-verification-only" "gemrelay:local"
```

该命令不挂载现有数据卷，仅使用测试容器自身的数据库。服务就绪后检查 `http://127.0.0.1:18083/` 与管理页面；结束容器后，`--rm` 会移除临时容器数据。

镜像构建与独立容器验证不能替代 Compose 编排验证。检查 Compose 的环境变量展开、项目名和具名卷行为时，仍需运行 Docker Compose v2。

---

继续阅读：[部署与配置](deployment.md) · [API 与模型](api.md) · [账号与代理](operations.md) · [排障](troubleshooting.md) · [开发](development.md)
