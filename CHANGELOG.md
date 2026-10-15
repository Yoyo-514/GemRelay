# 更新日志

本仓使用独立版本号，遵循[语义化版本](https://semver.org/lang/zh-CN/)。
原项目的历史版本见 [zexadev/gemini-web2api-go](https://github.com/zexadev/gemini-web2api-go/blob/main/CHANGELOG.md)。

## 0.1.0

基于 [zexadev/gemini-web2api-go](https://github.com/zexadev/gemini-web2api-go) 的首个独立版本。

### 功能

- 保留 OpenAI 兼容接口、MCP 搜索、账号与代理池、管理面板及数据库支持。
- 支持 Argon 模型和低、中、高思考档位，是否可用取决于上游账号权限。
- 增加严格模型校验，默认关闭。确认上游实际模型与请求不符时返回明确错误；实时流式已发送的内容无法撤回。
- 增加账号用量查询，在 Cookie 池显示 5 小时与每周额度、重置时间和刷新操作。
- 为模型校验失败提供独立的错误统计分类。

### 工程与界面

- 项目更名为 GemRelay，模块、命令、镜像及运行标识统一使用 `gemrelay`。
- 配置读取范围为显式路径和工作目录；默认数据使用 GemRelay 独立路径。
- 按职责拆分 Go 内部包，程序入口迁至 [cmd/gemrelay/](cmd/gemrelay/)，全部测试集中在 [tests/](tests/)。
- 管理前端按样式、公共组件和业务页面拆分，调整手机导航、列表和设置界面。
- 精简首页，详细说明迁入 [docs/](docs/)，英文文档集中在 [docs/en/](docs/en/)。
- 修正文档与前端提示中不符合当前实现的模型、流式、配置和额度说明。
- CI 使用常规 Go 格式检查、静态检查、测试和构建，镜像与二进制发布需先通过检查。
- 手动构建提供可下载产物；版本标签需与程序版本一致。

### 部署说明

- 源码与默认镜像构建要求 Go 1.27.2。
- 镜像定义位于 [build/package/Dockerfile](build/package/Dockerfile)，Compose 位于 [deployments/docker-compose.yml](deployments/docker-compose.yml)。
- Compose 默认构建本仓源码，要求显式设置管理员令牌；构建上下文排除本地凭据、数据库和测试产物。
- 默认数据库为 `./data/gemrelay.db`，Compose 使用独立的 `gemrelay-data` 卷；旧项目数据需要自行备份和处理。
