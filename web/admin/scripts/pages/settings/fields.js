// ── 运行时配置 ───────────────────────────────────────────────────────────────
// 字段定义跟后端 RuntimeConfig 一一对应；范围与后端 validateRuntimeConfig 一致，
// 前端只做提示，真正的校验在后端（浏览器里的限制随便绕）。
// 必须一一对应：saveRtCfg 只按这里的字段拼 PUT body，后端整体反序列化，漏在这里
// 列出的 RuntimeConfig 字段每次保存都会被解成零值冲掉（multi_turn 栽过，见 #27）。
// 按语义分组，组内自适应多列。分组的依据是"改这一项时在想什么"：
// 调产能 / 调容错 / 调怎么发请求 / 调留多少数据。
const RT_GROUPS = [
  {
     title: '出口流量控制',
    fields: [
       { k: 'per_ip_concurrent', label: '并发请求数', type: 'int', min: 0, max: 1000,
        hint: '同一时刻最多几个请求在飞，0=不限' },
       { k: 'per_ip_rpm', label: '每分钟限额（RPM）', type: 'int', min: 0, max: 10000,
        hint: '滚动 60 秒窗口上限，0=不限' },
       { k: 'per_ip_rph', label: '每小时限额（RPH）', type: 'int', min: 0, max: 100000,
       hint: '滚动 1 小时上限，0=不限。单出口突发记录为 80-180 次，默认取下沿 80。低速率部署可调高，10 次/分钟的静态 IP 曾在 110 分钟内完成 800 次请求' },
    ],
  },
  {
    title: '重试与超时',
    fields: [
      { k: 'retry_attempts', label: '重试次数', type: 'int', min: 1, max: 10,
        hint: '含首次；流式已吐字后不再重试' },
      { k: 'retry_delay_sec', label: '重试间隔（秒）', type: 'int', min: 0, max: 60 },
      { k: 'request_timeout_sec', label: '上游超时（秒）', type: 'int', min: 5, max: 600,
        hint: '新建网络客户端使用此值。已有客户端会保留创建时的超时；修改后重启服务可确保全部出口使用新值。' },
    ],
  },
  {
    title: '请求行为',
    fields: [
      { k: 'default_model', label: '默认模型', type: 'select',
        hint: '客户端没传 model 时用哪个' },
       { k: 'max_prompt_bytes', label: '提示词字节上限', type: 'int', min: 0, max: 10000000,
       hint: '默认 128000 UTF-8 字节。超限时，登录请求尝试将历史转为文本附件；匿名请求或转换后仍超限时返回 400 context_length_exceeded，上传失败会报错。0=关闭本地检查；该设置不改变上游上下文窗口大小。' },
      { k: 'impersonate', label: 'TLS 指纹', type: 'text',
        hint: 'chrome_146 / chrome_144 / firefox_147 / safari_16_0 …' },
      { k: 'gemini_bl', label: 'Gemini bl 版本', type: 'text',
        hint: '上游前端版本号。请求大面积失败时先看这里是不是过期了' },
      { k: 'gemini_bl_auto', label: '自动跟随 bl 版本', type: 'bool',
        hint: '每 6 小时从 /app 页面获取一次最新版本号并使用；获取失败或格式变化时沿用上方手动值' },
      { k: 'auto_delete_conversation', label: '自动删网页会话', type: 'bool',
         hint: '登录态单轮请求成功后尝试删除网页会话；删除失败时保留回复。多轮路径不执行自动删除。' },
      { k: 'temporary_chat', label: '临时对话', type: 'bool',
        hint: '使用网页端原生「临时对话」（inner[45]=1）：会话保留在临时模式，排除在账号会话列表与模型训练之外，并省去删除请求。仅登录态生效；开启「服务端多轮续接」时以多轮续接的持久会话为准' },
      { k: 'multi_turn', label: '服务端多轮续接', type: 'bool',
         hint: '复用 Gemini 会话，续接时只发送最新消息，历史保留在服务端。首轮、重启或历史不匹配时发送完整历史；续接失败会作废本地记录，下次客户端重试时重开。带附件或使用媒体生成模型时走单轮。' },
      { k: 'anon_first', label: '匿名优先', type: 'bool',
        hint: '默认关。开启后，不需要登录的模型在无附件请求中优先匿名执行，不占用账号额度；工具定义不影响该判断。Pro、3.8 Flash、Argon、扩展思考、媒体工具或附件仍使用账号。' },
      { k: 'quota_poll_interval_sec', label: '额度轮询间隔（秒）', type: 'int', min: 10, max: 3600,
        hint: '默认 60 秒。仅在 Cookie 池页面可见时轮询，最多三路并发；刷新失败时保留上次成功数据并标注缓存。' },
      { k: 'quota_aware_pick', label: '按额度选号', type: 'bool',
        hint: '按剩余额度分带选择账号，同带按健康度与最久未用顺序轮转。使用 20 秒内的快照，按需异步补查。默认关闭；模型权限由上游账号决定。' },
      { k: 'strict_model', label: '严格模型校验', type: 'bool',
        hint: '默认关。确认上游自报模型与请求不符时以错误结束：未开流返回 429，已开流发送额度不足错误事件。已发出的内容保留在客户端；缺失证据、未知模型和媒体工具放行，思考档位不参与判断。' },
    ],
  },
  {
    title: '代理与 cookie 的降级行为',
    fields: [
      { k: 'proxy_cooldown_min', label: '熔断冷却（分钟）', type: 'int', min: 0, max: 10080,
        hint: '代理连续失败 5 次后的冷却时长。历史测试约 106-121 分钟恢复，默认 120；0=一直停用，需在代理池页面手动重置' },
      { k: 'fallback_direct', label: '无可用代理时退回直连', type: 'bool',
        hint: '关：代理全部占满时返回 429，仅使用代理出口（默认）。开：可用性优先，上游会看到本机出口' },
      { k: 'fallback_anon', label: 'cookie 失效时降级匿名', type: 'bool',
        hint: '默认关。账号失效时先尝试其他账号；账号耗尽后，关闭时返回错误，开启时改用匿名。需要登录的模型可能被上游降级；严格模型校验可用于识别已确认的基础模型不匹配。' },
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
