# 限制与故障排查

[首页](../README.md) · [English](en/troubleshooting.md)

命令均从仓库根目录执行。

<a id="limitations"></a>

## 限制

- **单 IP 上限**：突发地打，实测 **80-180 次请求**后被重定向到 sorry 页（连接复用能多打约 60%：并发 10 时复用 172/177、每次新建 106/109）。但**平缓打几乎打不满**——静态 IP 上 10 次/分钟连打 800 次没被拦。默认 `per_ip_rph=80` 取的是区间下沿 → 要放大产能配代理池，或按低速率跑并调高限额
- **登录态功能**：生图、音乐、画布和视频均有实现，视频需要账号权限；深度研究仍未实现（多步异步流程）
- **Function calling**：prompt 级实现，模型不一定每次都按格式返回（OpenAI 真协议层我们做不到）
- **多模态**：读图/读视频要挂 cookie；生图/音乐/画布/生视频挂 cookie 可用，其中生视频还要 Pro/付费号
- **长上下文有两堵墙**：请求体约 13 万字节、附件约 16 万字节（后者是模型能看到的内容**总量**，切成多份附件不涨额度）。挂 cookie 只能把可用长度从 13 万提到约 16 万，真正的长对话仍需客户端自己压缩
- **token 数**：用 tiktoken 估算（Gemini 真 tokenizer 未公开），跟真值偏差 ±20% 以内
- **Cookie 池不自动摘除坏号**：请求成败会回写（只把 401/403 算作 cookie 的错，网络错误和 302 拦截不算），但失败到一定次数不会自动禁用，得看面板手动停。另外 `last_ok_at` 只说明"这个 cookie 参与的请求成功过"，不等于它仍然有效——cookie 过期后 Gemini 不报错，只是把你当匿名用户，纯文本请求照样 200
- **Chrome 导出的 cookie 可能续不了**：新版 Chrome 的 Device Bound Session Credentials 把会话绑在设备上，`__Secure-1PSIDTS` 换发会 401。用 Firefox 导出即可长期续命
- **流式输出**：Chat 和 Responses 均可转发增量正文；工具调用围栏要完整解析后再返回，Chat 才提供思考链增量

<a id="troubleshooting"></a>

## 故障排查

| 现象 | 多半是什么 | 怎么办 |
|---|---|---|
| 我们返回 **429** | 看错误码：`ip_slot_full` 是本地容量不足；`model_downgraded` 是严格校验拒绝了上游模型结果 | 前者调整限额或代理，后者检查账号额度与模型权限 |
| 面板诊断显示 **302 → `google.com/sorry/index`** | 这个出口 IP 被 Google 拦了（80-180 次请求后，取决于连接策略和出口质量） | 换出口/加代理。**是硬拦不是概率性**（被拦后 60 次探测零成功），原地重试没有意义 |
| 偶发空响应、面板记为上游拒绝 | 上游瞬时拒绝（`1155`），没有可预测阈值，跟频率/并发/累积次数都无关 | 重发一次通常就好。**降 RPM 解决不了**，实测跟频率无关 |
| 请求全部超时 | 本机到 `gemini.google.com` 不通 | 配代理（面板「代理池」或 `--proxy`）。注意**不读 `HTTPS_PROXY` 环境变量** |
| 启动即退出，报 `unable to open database file (14)` | 容器以 nonroot(uid 65532) 运行，而 bind mount 的宿主目录属主是 root，写不进去 | 改用具名卷（compose 默认已是），或 `sudo chown -R 65532:65532 ./data` |
| 选 `gemini-3.1-pro` 直接报错 | 没配 cookie 时它不暴露，这是故意的 | 挂 cookie（面板「设置」或「Cookie 池」）后即可用 |
| 挂了 cookie 后请求全部 502 | cookie 已失效，取不到 XSRF token | 重新导出 cookie。判据：请求 `gemini-3.1-pro` 若回报 3.5 Flash-Lite 就是失效了 |
| cookie 大约半小时就失效，保活没用 | Chrome 设备绑定会话，`__Secure-1PSIDTS` 换发 401 | 用 **Firefox** 重新登录再导出；点面板「保活」应能看到刷新了 `__Secure-1PSIDTS` |
| 面板打不开 / 401 | `--admin-token`（或 `ADMIN_TOKEN`）没对上 | token 留空则不鉴权，只有绑 127.0.0.1 时才可接受 |

Docker 用默认 bridge 网络时上游可能返回空内容（Google 拒绝某些 NAT 段）。本项目**没有复现过**，真遇到可以试 `network_mode: host` 验证是不是这个原因。

<a id="faq"></a>

## 常见问题（FAQ）

### 这和 Google 官方 Gemini API 有什么区别？需要 API Key 吗？

不需要。本项目不封装 `generativelanguage.googleapis.com`，而是直接反代 gemini.google.com 网页端的协议，
因此不需要 Google API Key 或官方 API 计费配额，但仍受网页端账号权限和额度限制。能力跟随已实现的网页协议：联网搜索、生图、音乐、画布和生视频
这里能用，网页端没有的（深度研究）这里也没有。匿名（不挂 cookie）就能跑文本模型。

### 和 Sophomoresty/gemini-web2api（Python）、其它叫 gemini-web2api 的 Go 项目是什么关系？

本仓基于 [zexadev/gemini-web2api-go](https://github.com/zexadev/gemini-web2api-go) 修改，保留 Go 网页协议代理，并调整目录结构、模型校验、额度查询和管理前端。
原项目早期协议工作参考过 [Sophomoresty/gemini-web2api](https://github.com/Sophomoresty/gemini-web2api) 的思路；这不表示本仓是该 Python 项目的移植版。

### Cookie 为什么大约半小时就失效？怎么长期用？

失效的那张票是 `__Secure-1PSIDTS`（约 30 分钟过期）。本项目会定时用哨兵 payload 换发它并做保活，
**Firefox 可避开 Chrome 的设备绑定问题，但不保证会话永久有效**。Chrome 新版本启用了设备绑定会话（DBSC），导出的 cookie 换发时会 401，
所以从 Chrome 导出的号半小时到几小时就死——换 Firefox 导出即可，不需要浏览器插件或挂机机器。

### 会被封号吗？封禁是按账号还是按 IP？

按**出口 IP + TLS 指纹**，不按账号。实测同一出口突发打 80～180 次会被重定向到 sorry 页，约两小时后自动恢复；
平缓地打（例如每分钟 10 次）连打 800 次没被拦。付费（Google AI Pro）**不会**提高这个阈值。
要放大产能靠代理池把请求分散到多个出口，而不是换账号。

### 免费账号和 Pro / 付费账号差在哪？

匿名（不挂 cookie）：`gemini-3.6-flash`、`gemini-3.5-flash-lite`，含联网搜索。
挂免费账号 cookie：额外解锁 `gemini-3.1-pro`、账号可用的文本思考档（`-thinking`）、读图 / 读视频、生图、音乐、画布。
付费账号才有：`gemini-3.8-flash`（免费号会被降级成 3.5 Flash-Lite）和生视频（`gemini-video` / `/v1/videos`）。

### 为什么每发一条消息，gemini.google.com 里就多出一个会话？

默认每个请求是独立会话。在面板「设置」勾选**服务端多轮续接**（`multi_turn`）后，同一串连续对话会复用同一个网页会话，
只发最新一句、历史留在服务端。不想让网页端堆会话有两种办法：勾**临时对话**（`temporary_chat`）让会话压根不进列表（更干净、不入训练、不用删，推荐），
或勾**自动删网页会话**（`auto_delete_conversation`）出完结果再删掉。两者都只对登录号有意义。

### 能当 Cursor / Dify / WorkBuddy 这类 agent 工具的后端吗？

能接（OpenAI 兼容），纯聊天和写代码没问题；但网页端**单次请求约 13 万字节封顶**（挂 cookie 约 16 万），
agent 工具一次塞几万字系统提示和历史时容易答非所问、看起来变笨——这是网页端模型的先天限制，不是接入方式的问题。
Function calling 是 prompt 级实现，模型不一定每次都按格式返回。

### 有 gemini-3.8-flash 吗？

有（`gemini-3.8-flash` 与 `gemini-3.8-flash-thinking`），需要付费 Gemini 账号；免费号请求它会被服务端降级成 3.5 Flash-Lite。
`gemini-3.7-flash` 作为别名保留，老客户端不受影响。

### 生图用的是哪个模型？能出 2K 吗？

`gemini-image` 走的是网页端的生图工具位，不锁模型：网页端当前用什么生图后端（如 Nano Banana 2），这里就自动跟着。
分辨率被网页端锁在约 100 万像素，没有 2K。

### 为什么在 Dify / 反向代理后面流式变成了一次性输出？

Chat 和 Responses 都支持增量正文。带工具时，围栏闸门会暂存工具调用块，其他正文仍可流式发送；Responses 暂不发送思考链事件。响应已带 `X-Accel-Buffering: no`，若经过反代仍被缓冲，请检查反代自身的缓冲设置。

### 支持 MySQL / PostgreSQL 吗？

支持。设环境变量 `SQL_DSN`（`mysql://user:pass@host:3306/db` 或 `postgres://user:pass@host:5432/db?sslmode=disable`），
不设默认 SQLite；建表自动完成，三种库共用一套 schema。单机用 SQLite 最省事，多实例共享一个池子才需要 MySQL / PG。

---

继续阅读：[部署与配置](deployment.md) · [API 与模型](api.md) · [账号与代理](operations.md) · [排障](troubleshooting.md) · [开发](development.md)
