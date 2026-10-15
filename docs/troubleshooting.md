# 限制与故障排查

[首页](../README.md) · [English](en/troubleshooting.md)

命令均从仓库根目录执行。

<a id="limitations"></a>

## 限制

- **单 IP 上限**：历史测试中，突发请求约 **80-180 次**后遇到 sorry 页；静态 IP 以 10 次/分钟完成过 800 次请求。默认 `per_ip_rph=80` 取突发区间下沿，可按出口表现和请求节奏调整。数据记录见[代理池](operations.md#proxies)
- **登录态功能**：生图、音乐、画布和视频均有实现，视频需要账号权限；深度研究的多步异步流程暂未实现
- **Function calling**：通过提示词约定输出格式，调用可靠性取决于模型遵循情况
- **多模态**：读图/读视频需要 Cookie；生图、音乐、画布和生视频同样需要 Cookie，其中生视频需要 Pro/付费号权限
- **长上下文**：请求体约 13 万字节；登录后转为附件可用约 16 万字节，附件按模型可见内容总量计算，拆分后的总额度相同。更长对话需要客户端压缩
- **token 数**：使用 tiktoken 估算（Gemini tokenizer 未公开），记录的偏差在 ±20% 以内
- **Cookie 账号状态**：请求结果会回写到账号，失败次数仅统计 401/403，网络错误和 302 拦截归为出口问题；账号停用由用户在面板操作。`last_ok_at` 记录请求成功时间；过期 Cookie 可能被按匿名处理，登录态有效性请通过「检测」确认
- **Cookie 续期**：新版 Chrome 的设备绑定会话可能在 `__Secure-1PSIDTS` 换发时返回 401。建议从 Firefox 导出，续期结果取决于上游策略
- **流式输出**：Chat 和 Responses 均可转发增量正文；工具调用围栏完整解析后以结构化结果返回，思考链增量仅在 Chat 提供

<a id="troubleshooting"></a>

## 故障排查

| 现象 | 多半是什么 | 怎么办 |
|---|---|---|
| 服务返回 **429** | `ip_slot_full` 表示本地容量不足；`model_downgraded` 表示严格校验发现模型不匹配 | 前者调整限额或代理，后者检查账号额度与模型权限 |
| 面板诊断显示 **302 → `google.com/sorry/index`** | 当前出口 IP 受 Google 风控限制 | 更换出口或等待恢复；被拦后的 60 次探测均失败，数据见[代理池](operations.md#proxies) |
| 偶发空响应、面板记为上游拒绝 | 上游瞬时拒绝（`1155`）；历史测试未发现与频率、并发或累计次数的关联 | 可重试一次；调低 RPM 在该测试中无效 |
| 请求全部超时 | 本机到 `gemini.google.com` 的连接失败 | 在「代理池」或通过 `--proxy` 配置代理；`HTTPS_PROXY` 环境变量会被忽略 |
| 启动即退出，报 `unable to open database file (14)` | 容器以 nonroot(uid 65532) 运行，而 bind mount 的宿主目录属主是 root，写不进去 | 改用具名卷（compose 默认已是），或 `sudo chown -R 65532:65532 ./data` |
| 选 `gemini-3.1-pro` 直接报错 | 该模型需要登录账号 | 在「Cookie 池」导入具备对应权限的账号 |
| 挂了 cookie 后请求全部 502 | cookie 已失效，取不到 XSRF token | 重新导出 cookie。判据：请求 `gemini-3.1-pro` 若回报 3.5 Flash-Lite 就是失效了 |
| cookie 大约半小时就失效，保活没用 | Chrome 设备绑定会话，`__Secure-1PSIDTS` 换发 401 | 用 **Firefox** 重新登录再导出；点面板「保活」应能看到刷新了 `__Secure-1PSIDTS` |
| 面板打不开 / 401 | 管理员令牌与 `--admin-token`（或 `ADMIN_TOKEN`）配置值不符 | 核对令牌；留空会关闭鉴权，仅限绑定 127.0.0.1 的本机部署使用 |

有反馈称 Docker 默认 bridge 网络可能因 NAT 出口被 Google 拒绝而返回空内容，此现象在本项目中尚未复现。遇到时可用 `network_mode: host` 作对照，检查是否与网络模式有关。

<a id="faq"></a>

## 常见问题（FAQ）

### 这和 Google 官方 Gemini API 有什么区别？需要 API Key 吗？

调用使用 gemini.google.com 的网页协议，无需 Google API Key 或官方 API 计费配额，账号权限和额度按网页端规则执行。
已实现联网搜索、生图、音乐、画布和生视频。两个文本模型支持匿名调用。

### 和 Sophomoresty/gemini-web2api（Python）、其它叫 gemini-web2api 的 Go 项目是什么关系？

本仓基于 [zexadev/gemini-web2api-go](https://github.com/zexadev/gemini-web2api-go) 修改，保留 Go 网页协议代理，并调整目录结构、模型校验、额度查询和管理前端。
早期协议工作参考过 [Sophomoresty/gemini-web2api](https://github.com/Sophomoresty/gemini-web2api) 的思路，本仓沿用 Go 实现路线。

### Cookie 为什么大约半小时就失效？怎么长期用？

`__Secure-1PSIDTS` 有效期约 30 分钟，服务会定时用哨兵 payload 换发并保活。
新版 Chrome 的设备绑定会话（DBSC）可能使导出后的 Cookie 换发返回 401，在半小时到几小时内失效。
建议从 Firefox 导出并交由服务保活，实际会话有效期取决于上游策略，详见 [Cookie 续期](operations.md#cookies)。

### 会被封号吗？封禁是按账号还是按 IP？

sorry 页拦截与出口 IP、TLS 指纹有关；付费账号（Google AI Pro）也受同类出口限制。
突发与恢复记录见[代理池](operations.md#proxies)。分散出口通过代理池实现，账号池负责账号轮转与权限选择。

### 免费账号和 Pro / 付费账号差在哪？

匿名（不挂 cookie）：`gemini-3.6-flash`、`gemini-3.5-flash-lite`，含联网搜索。
挂免费账号 cookie：额外解锁 `gemini-3.1-pro`、账号可用的文本思考档（`-thinking`）、读图 / 读视频、生图、音乐、画布。
付费账号才有：`gemini-3.8-flash`（免费号会被降级成 3.5 Flash-Lite）和生视频（`gemini-video` / `/v1/videos`）。

### 为什么每发一条消息，gemini.google.com 里就多出一个会话？

默认每个请求是独立会话。在面板「设置」勾选**服务端多轮续接**（`multi_turn`）后，同一串连续对话会复用同一个网页会话，
只发最新一句、历史留在服务端。登录态单轮请求可选择**临时对话**（`temporary_chat`），将会话排除在账号会话列表与模型训练之外，
或开启**自动删网页会话**（`auto_delete_conversation`），在生成成功后尝试删除。

### 能当 Cursor / Dify / WorkBuddy 这类 agent 工具的后端吗？

能接（OpenAI 兼容），纯聊天和写代码可用。网页端**单次请求约 13 万字节封顶**（登录后约 16 万），
agent 一次发送大量系统提示和历史时可能触及该限制并影响回答。
Function calling 依赖模型遵循提示词格式，接入后需验证实际任务表现。

### 有 gemini-3.8-flash 吗？

有（`gemini-3.8-flash` 与 `gemini-3.8-flash-thinking`），需要付费 Gemini 账号；免费号请求它会被服务端降级成 3.5 Flash-Lite。
`gemini-3.7-flash` 作为别名保留，老客户端不受影响。

### 生图用的是哪个模型？能出 2K 吗？

`gemini-image` 使用网页端生图工具位，后端随网页端变化，例如 Nano Banana 2。
当前分辨率上限约为 100 万像素，2K 输出暂不可用。

### 为什么在 Dify / 反向代理后面流式变成了一次性输出？

Chat 和 Responses 都支持增量正文。带工具时，围栏闸门会暂存工具调用块，其他正文照常转发；思考链事件仅在 Chat 提供。响应已带 `X-Accel-Buffering: no`，若经过反代仍被缓冲，请检查反代自身的缓冲设置。

### 支持 MySQL / PostgreSQL 吗？

支持。设环境变量 `SQL_DSN`（`mysql://user:pass@host:3306/db` 或 `postgres://user:pass@host:5432/db?sslmode=disable`），
默认为 SQLite；建表自动完成，三种库共用一套 schema。单实例可用 SQLite，多实例共享池数据可使用 MySQL / PostgreSQL。

---

继续阅读：[部署与配置](deployment.md) · [API 与模型](api.md) · [账号与代理](operations.md) · [排障](troubleshooting.md) · [开发](development.md)
