// ── 运行时配置 ───────────────────────────────────────────────────────────────
// 字段定义跟后端 RuntimeConfig 一一对应；范围与后端 validateRuntimeConfig 一致，
// 前端只做提示，真正的校验在后端（浏览器里的限制随便绕）。
// 必须一一对应：saveRtCfg 只按这里的字段拼 PUT body，后端整体反序列化，漏在这里
// 列出的 RuntimeConfig 字段每次保存都会被解成零值冲掉（multi_turn 栽过，见 #27）。
// 按语义分组，组内自适应多列。分组的依据是"改这一项时在想什么"：
// 调产能 / 调容错 / 调怎么发请求 / 调留多少数据，而不是按字段类型堆在一起。
const RT_GROUPS = [
  {
     title: '出口流量控制',
    fields: [
       { k: 'per_ip_concurrent', label: '并发请求数', type: 'int', min: 0, max: 1000,
        hint: '同一时刻最多几个请求在飞，0=不限' },
       { k: 'per_ip_rpm', label: '每分钟限额（RPM）', type: 'int', min: 0, max: 10000,
        hint: '滚动 60 秒窗口上限，0=不限' },
       { k: 'per_ip_rph', label: '每小时限额（RPH）', type: 'int', min: 0, max: 100000,
        hint: '滚动 1 小时上限，0=不限。实测单出口 80-180 次被拦，默认取下沿 80。按低速率跑可以调高很多——10 次/分钟连打 800 次没被拦过' },
    ],
  },
  {
    title: '重试与超时',
    fields: [
      { k: 'retry_attempts', label: '重试次数', type: 'int', min: 1, max: 10,
        hint: '含首次；流式已吐字后不再重试' },
      { k: 'retry_delay_sec', label: '重试间隔（秒）', type: 'int', min: 0, max: 60 },
      { k: 'request_timeout_sec', label: '上游超时（秒）', type: 'int', min: 5, max: 600 },
    ],
  },
  {
    title: '请求行为',
    fields: [
      { k: 'default_model', label: '默认模型', type: 'select',
        hint: '客户端没传 model 时用哪个' },
       { k: 'max_prompt_bytes', label: '提示词字节上限', type: 'int', min: 0, max: 10000000,
        hint: '超了直接返回 400 context_length_exceeded。上游的墙实测在约 13 万 UTF-8 字节，按字节算跟语言无关（同字节数下中英文表现一致，而 token 数差 1.9 倍）。不拦的话上游从尾部静默截断，被吃掉的正是刚问的那句。0=不限' },
      { k: 'impersonate', label: 'TLS 指纹', type: 'text',
        hint: 'chrome_146 / chrome_144 / firefox_147 / safari_16_0 …' },
      { k: 'gemini_bl', label: 'Gemini bl 版本', type: 'text',
        hint: '上游前端版本号。请求大面积失败时先看这里是不是过期了' },
      { k: 'gemini_bl_auto', label: '自动跟随 bl 版本', type: 'bool',
        hint: '每 6 小时从 /app 页面抓一次最新版本号，抓到就用抓到的。抓不到或格式不对就继续用上面钉的值' },
      { k: 'auto_delete_conversation', label: '自动删网页会话', type: 'bool',
         hint: '单轮请求成功后尝试删除网页会话，删除失败不影响回复。仅登录态生效；服务端多轮路径不执行自动删除。' },
      { k: 'temporary_chat', label: '临时对话', type: 'bool',
        hint: '走网页端原生「临时对话」（inner[45]=1）：会话压根不进账号的会话列表、不用于训练，比「自动删网页会话」更干净，还省掉那次删除请求。只登录态有意义；开了「服务端多轮续接」时不生效（临时对话不留列表，多轮要靠服务端留会话）' },
      { k: 'multi_turn', label: '服务端多轮续接', type: 'bool',
         hint: '复用 Gemini 会话，续接时只发送最新消息，历史保留在服务端。首轮、重启或历史不匹配时发送完整历史；续接失败会作废本地记录，下次客户端重试时重开。带附件或使用媒体生成模型时走单轮。' },
      { k: 'anon_first', label: '匿名优先', type: 'bool',
        hint: '这次请求不需要登录态能力时（纯文本、非思考、无工具、不带图）走匿名、不占用 cookie 账号，省账号额度；用到 3.1 Pro / 3.8 Flash / 扩展思考 / 生图 / 传文件这些才挑号。关掉则只要池里有号就一律用号（默认关）' },
      { k: 'strict_model', label: '严格模型校验', type: 'bool',
        hint: '默认关。上游自报模型与请求不符时明确失败：未开流返回 429，已开流发送额度不足错误事件。保留实时流式，因此报错前可能已发出部分正文；没有模型证据时放行。' },
    ],
  },
  {
    title: '代理与 cookie 的降级行为',
    fields: [
      { k: 'proxy_cooldown_min', label: '熔断冷却（分钟）', type: 'int', min: 0, max: 10080,
        hint: '代理连续失败 5 次后歇多久再放回池子。实测被拦的出口 106-121 分钟自恢复，默认 120；0=不恢复，要去代理池页面手动重置' },
      { k: 'fallback_direct', label: '无可用代理时退回直连', type: 'bool',
        hint: '关：直接返回 429，本机 IP 不会暴露给上游（默认）。开：可用性优先，但上游会看到本机真实出口' },
      { k: 'fallback_anon', label: 'cookie 失效时降级匿名', type: 'bool',
        hint: '关：直接报错，能立刻发现该换号了（默认）。开：继续跑，但 3.1 Pro 会被静默降级成 3.5 Flash-Lite、思考链消失，客户端看不出来' },
    ],
  },
  {
    title: '数据与日志',
    fields: [
      { k: 'retention_days', label: '明细保留（天）', type: 'int', min: 1, max: 3650,
        hint: '过期只删明细，聚合数据永久保留' },
      { k: 'log_requests', label: '打印请求日志', type: 'bool' },
    ],
  },
];
