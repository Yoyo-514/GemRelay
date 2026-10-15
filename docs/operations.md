# 管理面板、账号与代理

[首页](../README.md) · [English](en/operations.md)

命令均从仓库根目录执行。

<a id="admin"></a>

## 管理面板

`http://localhost:8083/admin`，用 `--admin-token` 登录。

- **概览** — 24h KPI + 请求量/P50 延迟双轴趋势图 + 模型/代理分组统计 + IP 限流用量 + 一键连通性诊断
- **请求记录** — 元数据明细、状态/模型筛选和分页
- **代理池** — 运行时增删改 + 启用/禁用 + 失败次数熔断（每代理是独立 IP slot）
- **Cookie 池** — 导入多个 Google 登录态账号，请求按**最久未用优先**自动轮转。每个账号一键「检测」是否仍是登录态，**失效的账号标红提示重导**；自动换发 `__Secure-1PSIDTS` + 每 10 分钟保活；每个账号粘住自己的出口。列表只显示脱敏摘要（cookie 数 / 关键项 / SAPISID 末 4 位 / 失败次数）
- **设置** — 运行时配置表单（修改超时后需重启以更新已有客户端）+ API Key 轮换 + 部署期配置只读展示

### 用量额度

Cookie 池的「用量额度」列逐账号显示 Gemini 网页端 **5 小时窗口 / 每周**两档剩余百分比与重置时间。
打开页面时查询，最多三路并发，后续通过「刷新额度」手动更新并跳过服务端 20 秒缓存。
停用账号也能查询；缺失档位显示「未提供」，查询失败显示错误原因。

也可调用 `GET /admin/api/quota[?account_id=N][&refresh=1]`，沿用管理接口鉴权。
不传账号时自动选择启用账号；失败返回 HTTP 200 与 `ok:false`、`error`，指定账号的失败也回填 `account_id`。

```json
{"ok":true,"account":"my-account","account_id":1,"direct":false,
 "five_hour":{"used_percent":63,"remain_percent":37,"reset_at":1792050755,"reset_in_sec":13504},
 "weekly":{"used_percent":13,"remain_percent":87,"reset_at":1792486355,"reset_in_sec":503104},
 "fetched_at":1792037251}
```

查询使用独立的只读路径，账号轮转、健康度、出口绑定和生成限流计数保持原值。
优先使用账号绑定的可用代理；直连回退由 `fallback_direct` 控制。
成功结果按实际账号、Cookie 与出口身份缓存 20 秒，同身份并发查询合并；失败时清除缓存。
数据来自逆向的网页端 RPC `jSf9Qc`，解析判据参考 MIT 项目
[Nagi-ovo/voyager](https://github.com/Nagi-ovo/voyager) 的 `usageParsing.ts`。
该端点与生成接口独立，可能因上游改版或账号尚未开放额度页面而查询失败。

面板前端按职责拆为 HTML 外壳、[样式](../web/admin/styles/)和[脚本](../web/admin/scripts/)，
Chart.js 与全部资源随二进制嵌入，支持内网或离线加载，直接使用原生脚本和样式。

面板使用相对地址，支持反代子路径。将 `https://example.com/gemini/` 转发到本服务的 `/`，
即可通过 `https://example.com/gemini/admin` 访问。`/admin` 会以 301 跳转到 `admin/`，
统一资源相对路径的解析目录。

<a id="cookies"></a>

## Cookie（可选）

挂 Google 账号 cookie 后请求走登录态，多出来的能力是 **`gemini-3.1-pro` + 思考链**
（见[思考链说明](api.md#reasoning)）、**读图 / 读视频**、**生图（`gemini-image`）/ 音乐（`gemini-music`）
/ 画布（`gemini-canvas`）/ 生视频（`gemini-video`，要 Pro 号）**。历史测试在免费账号上
连续调用 6 次，均回报 `3.1 Pro`。

深度研究（多步异步）仍**未实现**。

登录请求需要 XSRF token，服务会自动从 Gemini 页面获取、按 Cookie 缓存，并在过期后重新获取。
缺少该 token 时登录请求返回 400，匿名请求则照常处理。

1. 浏览器登录 [gemini.google.com](https://gemini.google.com)
2. DevTools (F12) → Application → Cookies → `https://gemini.google.com`
3. 复制：`SID` / `HSID` / `SSID` / `APISID` / `SAPISID` / `__Secure-1PSID` / `__Secure-1PSIDTS`
4. 粘进面板「Cookie 池 → 添加账号」；或写成 `cookie.txt` 后启动加
   `--cookie-file cookie.txt`（启动时导入池子，之后从面板管理）：
```
SID=...; HSID=...; SSID=...; APISID=...; SAPISID=...; __Secure-1PSID=...; __Secure-1PSIDTS=...
```

也支持 JSON 形式 `{"cookie": "SID=...; ...", "sapisid": "..."}`。
`SAPISID` 为必需项，用于计算 `SAPISIDHASH` 授权头。

Cookie 统一由「Cookie 池」管理。每个请求按最久未用优先选择启用账号，选中后推进轮转。
池中有启用账号时，`gemini-3.1-pro` 会出现在模型列表中。

账号的「失败次数 / 最近成功」随请求更新。失败次数仅统计 401/403；网络错误、代理失败和
Google 302 拦截归为网络或出口问题，以区分凭据失效与代理质量。

「最近成功」记录使用该 Cookie 的请求成功时间；过期 Cookie 可能被上游按匿名处理，文本请求仍可成功。
确认登录态请使用列表中的「检测」按钮，结果会区分“登录态有效”与“过期/无效”。

**服务会尝试自动续期 Cookie**，能否续期仍取决于上游会话与设备绑定。当前有两条刷新路径：

- 上游几乎每个响应都会刷新 `SIDCC` / `__Secure-1PSIDCC` / `__Secure-3PSIDCC`，服务将其合并写回账号。
- 每 10 分钟（启动后 15 秒先刷一次）往 `accounts.google.com/RotateCookies` 打保活：
  先用哨兵 payload 换发 `__Secure-1PSIDTS`（有效期约 30 分钟，过期后会转为匿名态），
  再通过浏览器 iframe 流程刷新 SIDCC。同一账号的尝试间隔至少 60 秒，以避免 429。

> 建议从 Firefox 导出 Cookie，其会话可通过服务保活尝试续期，实际有效期取决于上游策略。
> 新版 Chrome 的 Device Bound Session Credentials 会将会话绑定到设备，导出后可能在半小时到几小时内失效，`*PSIDTS` 换发失败。
> 使用 Chrome 时，请在全新隐私窗口中登录，导出后立即关闭窗口，让服务接管后续会话轮转。

账号绑定首次使用的出口，出口不可用时才切换。这样可避免账号与代理独立轮转时，
同一账号频繁跨 IP 请求，被 Google 识别为账号共享。

账号失效后，服务会尝试池中的下一个账号。

<a id="proxies"></a>

## 代理池

本节数据为历史测试记录，实际容量和恢复时间随出口与上游策略变化。多个代理地址共用公网出口时，也会共享该 IP 的上游额度。


历史测试中，单 IP 突发请求约 **80–180 次**后被重定向到 `google.com/sorry/index`，
结果受连接策略、出口质量和请求节奏共同影响：

| 出口 | 连接策略 | 并发 | 节奏 | 被拦时的成功次数 |
|---|---|---|---|---|
| 住宅 | 复用连接池 | 10 | 无间隔 | 151 / 172 / 177 |
| 住宅 | 复用连接池 | 3 | 无间隔 | 103 / 111 |
| 住宅 | 每次新建连接 | 10 | 无间隔 | 106 / 109 |
| 住宅 | 每次新建连接 | 1 | 间隔 24s | 81 / 166 |
| 静态 | 复用连接池 | 10 | 无间隔 | 188 |
| 静态 | 复用连接池 | 1 | **10 次/分钟** | **800 次没被拦** |

判据只认 302 → `/sorry/`，出口都预筛过。

连接复用组的成功次数高约 60%：两组并发均为 10，同时开始并运行 80 秒，以减少出口漂移影响；
复用连接为 172/177 次，每次新建连接为 106/109 次。

请求节奏在静态 IP 测试中影响明显：同一出口突发请求在 188 次后被拦，改为 10 次/分钟后，
110 分钟内完成 800 次且全部通过。默认 `per_ip_rph=80` 取突发区间下沿，低速率部署可结合出口表现调高。

> 早前这里写「放慢节奏没用」，判据是住宅出口上突发档 103-177 与慢速档 81-166 几乎
> 完全重叠。后续发现住宅出口退化掩盖了节奏影响：8 个预筛出口中，6 个在慢速测试中途失败率超过 40%。
> 改用静态 IP 排除该因素后，节奏的作用更明显。

部分失败请求已经到达 Google 并计入额度。慢速组发送 195 次、成功 166 次，实际请求量比成功数高 17%；
重试也应计入出口容量规划。

被拦后的两次独立复测各发送 30 次探测、间隔 20 秒、持续约 10 分钟，合计 60 次均失败；
继续探测时，在 **106–121 分钟**恢复。代理池默认 120 分钟的熔断冷却参考了这组记录。

另一组对照使用 10 个 IP、每个计划请求 50 次，记录的 418 次请求均通过 Google。

在管理面板「代理池」添加代理后，每个代理作为独立的本地 IP slot，分别计算并发/RPM/RPH。
N 个代理对应 N 份本地限额，上游容量还取决于实际公网出口。

支持的代理协议：
- `http://user:pass@host:port`
- `https://user:pass@host:port`
- `socks5://user:pass@host:port`
- `socks5h://user:pass@host:port`（远程 DNS 解析，绕开本地 DNS 污染）

**自动调度规则**：
- 配置代理后默认仅走代理池；开启 `fallback_direct` 才允许退回直连
- 失败 5 次自动熔断（管理面板可手动重置）
- 全部代理满时在本地返回 HTTP 429，待空位释放后重试

出口统一由代理池管理，`--proxy` / `config.json` 用于启动导入。
`HTTPS_PROXY` / `ALL_PROXY` 环境变量会被忽略，使实际出口与面板展示保持一致。

## 指纹模拟

直连使用 [bogdanfinn/tls-client](https://github.com/bogdanfinn/tls-client)，按 `impersonate` 选择浏览器握手档位，默认 Chrome 146。浏览器指纹模拟仍受 Google 风控和出口限流约束。

代理连接使用 Go 标准库 `net/http` + `http.ProxyURL`，TLS 指纹来自 Go 客户端；应用层请求头
（`Sec-CH-UA` / `Sec-Fetch-*` / `User-Agent`）按所选浏览器档位设置。
HTTP CONNECT 建立隧道，TLS 握手发生在客户端与 Google 之间。JA3 回显测试中，同一代理下
stdlib 与 tls-client 的结果不同，stdlib 直连与过代理的结果相同。

同时开始的对照中，tls-client Chrome_146 完成 111 次，Go stdlib 完成 103 次，相差 8 次；
同配置出口之间差异可达 36%（151 vs 111）。这组结果中，出口质量的影响大于 TLS 客户端差异；
代理路径是否采用 Chrome 指纹，应结合长期账号画像等需求判断。

测试还记录到：Python urllib 等普通 SDK 请求触发风控时返回 HTTP 429，模拟 Chrome 的请求返回 HTTP 302 并跳转到 `google.com/sorry/index`（CAPTCHA 验证页）。两者均表示上游风控介入。

<a id="privacy"></a>

## 隐私

- 数据库中的请求记录仅保存元数据：模型、代理 ID、延迟、token 数、状态码、错误类型
- 历史 `prompt_preview` / `response_preview` 列从老版本迁移时自动 DROP
- Token 数在分词器初始化成功时使用 [tiktoken-go](https://github.com/pkoukk/tiktoken-go) cl100k_base；加载失败时退回 UTF-8 字节数除以 4。两种结果都只是对 Gemini 用量的本地估算
- 已观察的 Gemini 网页响应缺少 token/usage 字段，因此用量由本地估算

---

继续阅读：[部署与配置](deployment.md) · [API 与模型](api.md) · [账号与代理](operations.md) · [排障](troubleshooting.md) · [开发](development.md)
