# 目录与依赖约束

本项目使用一个 Go module、一个可执行程序，继续以单二进制部署。顶层目录参考
[golang-standards/project-layout](https://github.com/golang-standards/project-layout/blob/master/README_zh-CN.md)，
仅采用实际需要的部分；该布局不是 Go 官方标准。

## 职责

| 位置 | 职责 |
|---|---|
| [程序入口](../cmd/gemini-web2api/main.go) | 调用启动装配，不包含业务实现 |
| [启动装配](../internal/app/) | 参数覆盖、初始化顺序、监听启动、后台任务调度 |
| [配置](../internal/config/) | 启动配置、运行时快照、版本和现有请求日志开关 |
| [HTTP adapter](../internal/httpapi/) | 路由、鉴权、协议编解码、SSE、管理页面和错误映射 |
| [生成编排](../internal/gateway/) | 账号与出口协调、重试、多轮、消息与工具转换、视频任务 |
| [Gemini 协议](../internal/gemini/) | 模型请求参数、上游传输协议、解析、上传、媒体与凭据刷新 |
| [资源池](../internal/pool/) | 账号和代理持久化、选择、绑定、容量、健康状态 |
| [Cookie 规则](../internal/cookies/) | 归一化、提取、身份比较和刷新合并，无上游网络调用 |
| [存储](../internal/storage/) | 连接生命周期、现有 schema 与升级语句、方言、事务、统计 |
| [基础传输](../internal/transport/) | TLS 配置、浏览器请求头和客户端缓存，不挑选账号 |
| [管理资源](../web/admin/) | 页面、Chart.js 和嵌入声明，不放后端业务逻辑 |

## 依赖方向

- 启动装配连接各包；其他内部包不反向导入启动装配。
- HTTP adapter 调用生成编排、资源池和存储；业务包不导入 HTTP adapter。
- 生成编排协调资源池与上游协议；资源池和上游协议不互相导入。
- 资源池调用存储和 Cookie 规则；存储不依赖资源池、上游协议或 HTTP。
- 上游协议调用 Cookie 规则和基础传输；基础传输不访问资源池或数据库。
- 配置快照不自行加载数据库或解析模型可用性。持久化与模型校验由生成编排协调。

账号检测和保活跨越账号状态与上游网络，放在生成编排中。资源池决定使用哪个账号和出口；
协议包负责发出指定凭据与出口的请求；编排在结果返回后协调写回。这样避免循环导入，
同时保留原有账号—出口绑定、身份保护及失败归责规则。

数据库连接和方言继续属于同一存储 module。SQLite、MySQL、PostgreSQL 的差异集中处理，
不新增通用仓储框架，不把现有 schema 改成另一套迁移机制。

## 状态与生命周期

本轮保留现有单进程的状态和缓存生命周期，不将项目改造成可在一个进程中任意创建多个实例的框架。
启动配置仅在初始化时修改；运行时配置通过加锁的快照读取和原子发布更新，锁与存储值保持私有。
数据库仍在进程内惰性打开，SQLite 保持单连接，结束时可关闭连接。

凭据缓存、传输缓存和多轮状态的失效策略不因目录迁移改变。后台任务执行顺序与周期也保持原样。
视频任务由编排包管理；HTTP 读取状态快照，不直接共享可变任务字段或访问任务锁。

## 测试

所有测试与测试辅助代码都位于 [测试根目录](../tests/)，生产目录不保留测试文件。
这是本项目的明确约定；参考布局使用单数测试目录，本项目采用复数命名。

- 测试按职责分组，跨账号、出口、存储与启动语义的回归放在 [集成测试](../tests/integration/)。
- [公共辅助](../tests/testutil/) 为各测试进程独立创建临时 SQLite，清除生产连接串的环境覆盖，关闭连接后清理目录。
- 通过导出业务能力和可观察输出测试，不公开私有锁、状态表或正则来满足测试访问。
- 管理输出通过请求记录器验证，不启动本地服务器，也不替换生产函数。
- 数据库故障回归继续使用真实 SQLite 约束、触发器和事务，覆盖写入失败及完整回滚。
- [真实上游探针](../tests/probes/) 默认跳过，仅在显式设置其环境变量时运行。
- 解析 fixture 跟随对应测试，例如 [搜索解析数据](../tests/gemini/testdata/)。

在仓库根目录运行：

```sh
go build ./...
go test ./... -count=1
go test ./... -shuffle=on -count=3
go vet ./...
```

需要生产包覆盖率时，应显式设置覆盖范围；默认覆盖率只统计测试所在的包：

```sh
go test ./tests/... -coverpkg=./internal/...,./web/... -coverprofile=coverage.out
```

## 路径迁移

| 原位置 | 当前位置 |
|---|---|
| 根目录入口 | [命令入口](../cmd/gemini-web2api/main.go) |
| 根目录配置模板 | [配置模板](../configs/config.example.json) |
| 根目录容器定义 | [镜像构建定义](../build/package/Dockerfile) |
| 根目录部署定义 | [Compose 配置](../deployments/docker-compose.yml) |
| 应用包内的管理页面 | [管理页面资源](../web/admin/) |
| 应用包内的测试与 fixture | [测试根目录](../tests/) |

从仓库根目录构建可执行程序或镜像：

```sh
go build -o gemini-web2api-go ./cmd/gemini-web2api
docker build -f build/package/Dockerfile -t gemini-web2api-go .
docker compose -f deployments/docker-compose.yml up -d
```

源码 Compose 构建需要取消部署文件中构建配置块的注释。构建上下文仍是仓库根目录，
配置中的容器定义路径相对于该上下文；相对绑定挂载路径则相对于部署文件所在目录。
从新部署文件启动已有部署前，应保留原有 Compose 项目名或显式指定原项目名，
避免为原数据卷创建另一个项目前缀。不要删除原卷。

独立下载的 Compose 文件仍可直接拉取现成镜像；其源码构建配置需要完整仓库。
配置模板迁移不会改变默认配置查找位置、数据库默认路径、配置键或环境变量优先级。
HTTP 路由、响应字段、数据库结构和网络回退策略保持原有约定。
