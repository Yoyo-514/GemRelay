# 管理面板、账号与代理

[首页](../README.md) · [English](en/operations.md)

命令均从仓库根目录执行。

<a id="admin"></a>

## 管理面板

`http://localhost:8083/admin`，用 `--admin-token` 登录。

- **概览** — 24h KPI + 请求量/P50 延迟双轴趋势图 + 模型/代理分组统计 + IP 限流用量 + 一键连通性诊断
- **请求记录** — 明细列表（仅元数据，无 prompt/response 内容），状态/模型筛选 + 分页
- **代理池** — 运行时增删改 + 启用/禁用 + 失败次数熔断（每代理是独立 IP slot）
- **Cookie 池** — 导入多个 Google 登录态账号，请求按**最久未用优先**自动轮转。每个账号一键「检测」是否仍是登录态，**失效的账号标红提示重导**；自动换发 `__Secure-1PSIDTS` + 每 10 分钟保活；每个账号粘住自己的出口。列表只显示脱敏摘要（cookie 数 / 关键项 / SAPISID 末 4 位 / 失败次数）
- **设置** — 运行时配置表单（修改超时后需重启以更新已有客户端）+ API Key 轮换 + 部署期配置只读展示

### 用量额度

Cookie 池的「用量额度」列逐账号显示 Gemini 网页端 **5 小时窗口 / 每周**两档剩余百分比与重置时间。
打开页面时查询，最多三路并发；不定时轮询。「刷新额度」会跳过服务端 20 秒缓存。
停用账号也能查询；缺失档位显示「未提供」，查询失败不会显示为 0%。

也可调用 `GET /admin/api/quota[?account_id=N][&refresh=1]`，沿用管理接口鉴权。
不传账号时自动选择启用账号；失败返回 HTTP 200 与 `ok:false`、`error`，指定账号的失败也回填 `account_id`。

```json
{"ok":true,"account":"my-account","account_id":1,"direct":false,
 "five_hour":{"used_percent":63,"remain_percent":37,"reset_at":1792050755,"reset_in_sec":13504},
 "weekly":{"used_percent":13,"remain_percent":87,"reset_at":1792486355,"reset_in_sec":503104},
 "fetched_at":1792037251}
```

查询走旁路：不推进账号轮转，不写健康度或账号出口绑定，不占生成请求的限流 slot。
优先使用账号绑定的可用代理；无健康代理时遵守 `fallback_direct`，不会绕过关闭的直连回退开关。
成功结果按实际账号、Cookie 与出口身份缓存 20 秒，同账号并发查询合并；失败不缓存。
数据来自逆向的网页端 RPC `jSf9Qc`，解析判据参考 MIT 项目
[Nagi-ovo/voyager](https://github.com/Nagi-ovo/voyager) 的 `usageParsing.ts`。
上游改版或账号未开放额度页面时可能不可用，不影响生成接口。

面板前端按职责拆为 HTML 外壳、[样式](../web/admin/styles/)和[脚本](../web/admin/scripts/)，
Chart.js 与全部资源随二进制 embed，**不走 CDN**——内网/离线部署也能开，不需要前端构建工具。

**反代到子路径不用额外配置**：面板里的地址全是相对的，把 `https://example.com/gemini/`
转发到本服务的 `/` 就能用 `https://example.com/gemini/admin` 打开。（访问不带尾斜杠的
`/admin` 会 301 到 `admin/`——相对地址按文档 URL 的目录解析，两种形式差一层，统一一下
才不会解析错。）

<a id="cookies"></a>

## Cookie（可选）

挂 Google 账号 cookie 后请求走登录态，多出来的能力是 **`gemini-3.1-pro` + 思考链**
（见[思考链说明](api.md#reasoning)）、**读图 / 读视频**、**生图（`gemini-image`）/ 音乐（`gemini-music`）
/ 画布（`gemini-canvas`）/ 生视频（`gemini-video`，要 Pro 号）**。免费账号实测可用，
连打 6 次全部回报 `3.1 Pro`。

深度研究（多步异步）仍**未实现**。

> 带 cookie 的请求必须额外携带一个 XSRF token，本项目会自动从 Gemini 页面取并按
> cookie 缓存、过期自动重取，无需配置。（这一步缺了会导致**所有**请求 400，
> 匿名反而不受影响——早期版本踩过这个坑。）

1. 浏览器登录 [gemini.google.com](https://gemini.google.com)
2. DevTools (F12) → Application → Cookies → `https://gemini.google.com`
3. 复制：`SID` / `HSID` / `SSID` / `APISID` / `SAPISID` / `__Secure-1PSID` / `__Secure-1PSIDTS`
4. 粘进面板「Cookie 池 → 添加账号」；或写成 `cookie.txt` 后启动加
   `--cookie-file cookie.txt`（启动时导入池子，之后从面板管理）：
```
SID=...; HSID=...; SSID=...; APISID=...; SAPISID=...; __Secure-1PSID=...; __Secure-1PSIDTS=...
```

JSON 形式 `{"cookie": "SID=...; ...", "sapisid": "..."}` 也吃。带 `SAPISID` 的请求会自动
算 `SAPISIDHASH` 授权头，所以这一项不能少。

**cookie 只有「Cookie 池」这一个入口**：每个请求按最久未用优先挑一个 enabled 账号，
挑中即推进轮转。池子里有 enabled 账号，`gemini-3.1-pro` 就会出现在模型列表里。

账号的「失败次数 / 最近成功」会随请求自动更新。判据是**只把 401/403 算作 cookie 的错**：
网络错误、代理失败、被 Google 拦（302）一律不计——住宅代理的出口退化率很高，把这些算
进去会让失败次数变成代理噪音，好 cookie 反而被记成失败最多的那个。

注意「最近成功」只说明这个 cookie 参与的请求成功过，**不等于 cookie 仍然有效**：cookie
过期后 Gemini 不报错，只是把你当匿名用户。要确认有效性点列表里的「检测」按钮，它区分
"登录态有效"和"过期/无效"，不用发一次真实对话去试。

**服务会尝试自动续期 Cookie**，能否续期仍取决于上游会话与设备绑定。当前有两条刷新路径：

- 上游几乎每个响应都会刷新 `SIDCC` / `__Secure-1PSIDCC` / `__Secure-3PSIDCC`，我们收下并写回账号。
- 每 10 分钟（启动后 15 秒先刷一次）往 `accounts.google.com/RotateCookies` 打保活：
  先用哨兵 payload 换发 `__Secure-1PSIDTS`（约 30 分钟过期的那张票，不刷就会被当匿名），
  再走浏览器 iframe 那条刷 SIDCC。同一账号 60 秒内不重复打，避免 429。

> **请用 Firefox 导出 cookie。** Chrome 新版本开了 Device Bound Session Credentials，
> 导出的会话绑在这台设备上，`*PSIDTS` 续不了，大概半小时到几小时就死。Firefox 没有
> 这层绑定，同一套保活可尝试续期，但不保证会话永久有效。用 Chrome 的话：开一个全新的隐私窗口登录、立刻导出、
> **马上关掉那个窗口**，不要让浏览器自己再去轮转同一份会话。

**每个账号会粘住自己的出口**。cookie 池和代理池如果各自独立轮转，同一个 Google 账号会
从几十个不同 IP 发出请求，这在 Google 眼里是账号共享的典型特征。账号首次用到哪个出口就
绑定下来，出口不可用了才换。

**一个号挂了会自动换下一个**，不让整个请求陪葬——否则池子越大越容易踩雷。

<a id="proxies"></a>

## 代理池（白嫖路线核心）

本节数据来自原项目的历史测试，不是容量或恢复时间保证。多个代理地址如果共用一个公网出口，仍会共同消耗该 IP 的上游额度。


**为什么需要**：单 IP 突发地打到一定次数就会被重定向到 `google.com/sorry/index`。
这个次数是 **80-180**，跨度很大，由**连接策略、出口质量和请求节奏**共同决定：

| 出口 | 连接策略 | 并发 | 节奏 | 被拦时的成功次数 |
|---|---|---|---|---|
| 住宅 | 复用连接池 | 10 | 无间隔 | 151 / 172 / 177 |
| 住宅 | 复用连接池 | 3 | 无间隔 | 103 / 111 |
| 住宅 | 每次新建连接 | 10 | 无间隔 | 106 / 109 |
| 住宅 | 每次新建连接 | 1 | 间隔 24s | 81 / 166 |
| 静态 | 复用连接池 | 10 | 无间隔 | 188 |
| 静态 | 复用连接池 | 1 | **10 次/分钟** | **800 次没被拦** |

判据只认 302 → `/sorry/`，出口都预筛过。

**连接复用值约 60%**：并发钉死 10、两臂同时起跑、全程只跑 80 秒（短到出口来不及
漂移），复用连接 172/177，每次新建 106/109。

**平缓节奏比什么都管用**：同一个静态 IP，突发打在 188 次被拦，改成 10 次/分钟连打
**800 次、跨 110 分钟一次没被拦**。所以默认 `per_ip_rph=80` 是很保守的下沿，
明确按低速率跑的部署可以调高很多。

> 早前这里写「放慢节奏没用」，判据是住宅出口上突发档 103-177 与慢速档 81-166 几乎
> 完全重叠。那个观察没错，但归因错了：住宅出口跑久了自己会退化（8 个预筛干净的
> 出口跑慢节奏，6 个中途失败率超 40%），退化盖过了节奏的影响。换静态 IP 排除掉
> 这个混淆项之后，节奏的作用非常明显。

**代理失败会提前消耗额度**：链路越脏上限来得越早，因为那些"失败"的请求有一部分其实
已经到达 Google 并被计数（慢速档实际发出 195 次才换到 166 次成功，真实消耗高 17%）。
别指望靠重试失败请求多榨产能。

**被拦之后是硬拦，约两小时自动恢复。** 两次独立复测各探测 30 次、间隔 20s、
跨约 10 分钟，合计 60 次零成功；继续探测到 **106-121 分钟**之间恢复正常。
代理池的熔断冷却默认取 120 分钟就是照这个来的。

对照：10 个 IP 各打 50 次（418 请求）零次 Google 拒绝。默认 `per_ip_rph=80` 落在突发
档实测区间的下沿。

**怎么解决**：在管理面板「代理池」页面加多个代理，**每个代理是一个独立的 IP slot**，享有独立的并发/RPM/RPH 配额。N 个代理 = N 倍总容量。

支持的代理协议：
- `http://user:pass@host:port`
- `https://user:pass@host:port`
- `socks5://user:pass@host:port`
- `socks5h://user:pass@host:port`（远程 DNS 解析，绕开本地 DNS 污染）

**自动调度规则**：
- 配了代理后，**默认不会退回直连**；只有开启 `fallback_direct` 才允许该回退
- 失败 5 次自动熔断（管理面板可手动重置）
- 全部代理满 → 返回 HTTP 429（不消耗 Google 配额，等空位再重试）

**不读 `HTTPS_PROXY` / `ALL_PROXY` 环境变量。** 代理只从代理池里取
（`--proxy` / `config.json` 只是启动时往池子里播种）——否则宿主机上一个随手 export 的变量会悄悄改变
出口 IP，而面板显示的还是直连，排查时会误判。

## 指纹模拟

直连使用 [bogdanfinn/tls-client](https://github.com/bogdanfinn/tls-client)，按 `impersonate` 选择浏览器握手档位，默认 Chrome 146。这不保证通过 Google 风控，也不免除出口限流。

代理连接使用 Go 标准库 `net/http` + `http.ProxyURL`；应用层请求头（`Sec-CH-UA` / `Sec-Fetch-*` / `User-Agent`）仍按所选浏览器档位设置，但不会因此获得该浏览器的 TLS 指纹。

**注意走代理时 TLS 指纹是我们自己的，不是出口节点的。** HTTP CONNECT 只建隧道，TLS
是我们跟 Google 端到端握的——JA3 回显实测：同一代理下 stdlib 和 tls-client 的 JA3
不同（代理没改写），而 stdlib 直连和过代理的 JA3 完全相同（代理层不介入 TLS）。所以
**配了代理，暴露给 Google 的就是 Go 标准库的指纹而不是 Chrome 146 的**。

但这不影响封禁阈值：同时起跑的对照里 tls-client Chrome_146 打到 111 次、Go stdlib
103 次，差 8 次，而同配置下出口之间的方差能到 36%（151 vs 111）。想让代理路径也走
Chrome 指纹得有别的理由（比如担心长期账号画像），不能指望换来产能。

**实测意外**：朴素 SDK 调用（如 Python urllib）触发风控时拿到 **HTTP 429**；伪装成 Chrome 后触发风控拿到 **HTTP 302** 跳转到 `google.com/sorry/index`（CAPTCHA 验证页）。两者都表明上游风控介入；单凭 302 不能证明客户端已通过浏览器验证。

<a id="privacy"></a>

## 隐私

- **Prompt 和 response 内容永不入库**——只存元数据：模型、代理 ID、延迟、token 数、状态码、错误类型
- 历史 `prompt_preview` / `response_preview` 列从老版本迁移时自动 DROP
- Token 数在分词器初始化成功时使用 [tiktoken-go](https://github.com/pkoukk/tiktoken-go) cl100k_base；加载失败时退回 UTF-8 字节数除以 4。两种结果都只是对 Gemini 用量的本地估算
- Gemini 网页端不返回真 token 数（实测响应里没有任何 token/usage 字段，只能本地估算）

---

继续阅读：[部署与配置](deployment.md) · [API 与模型](api.md) · [账号与代理](operations.md) · [排障](troubleshooting.md) · [开发](development.md)
