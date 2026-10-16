package config

import (
	"sync"
)

// RuntimeConfig 是面板里能随时改、改完立刻生效的那部分配置。
//
// 部署期配置（监听地址、DB 路径、admin token、API key、cookie 文件）**不在这里**：
// 那些改了本来就要重启进程，放 docker-compose.yml 的 environment / command 更合适，
// 也避免把凭证放进一个网页表单。这里只放调优参数——为了改个超时重启一次服务太蠢。
//
// 取值优先级：面板改过的（存 kv 表） > config.json / CLI flag > 内置默认。
//
// 加字段必读：面板保存走整体反序列化（admin.go 的 Decode(&RuntimeConfig)），而前端
// saveRtCfg 只按管理脚本中的 RT_GROUPS 拼 PUT body。新加的字段必须同步进字段清单，
// 否则它不在 body 里、每次点「保存并生效」都被解成零值冲掉（multi_turn 就这么被静默
// 重置过，连 config.json 里设的都白设，见 #27）。
type RuntimeConfig struct {
	RetryAttempts   int    `json:"retry_attempts"`
	RetryDelaySec   int    `json:"retry_delay_sec"`
	RequestTimeout  int    `json:"request_timeout_sec"`
	DefaultModel    string `json:"default_model"`
	PerIPConcurrent int    `json:"per_ip_concurrent"`
	PerIPRPM        int    `json:"per_ip_rpm"`
	PerIPRPH        int    `json:"per_ip_rph"`
	RetentionDays   int    `json:"retention_days"`
	LogRequests     bool   `json:"log_requests"`
	Impersonate     string `json:"impersonate"`
	GeminiBL        string `json:"gemini_bl"`
	// ProxyCooldownMin 是代理连续失败熔断后隔多久放回池子，单位分钟。
	// 0 = 不恢复（熔断即永久除名，要手动重置）。默认按实测的封禁恢复时长取 120。
	ProxyCooldownMin int `json:"proxy_cooldown_min"`
	// FallbackDirect 决定代理池一个出口都用不上时是退回直连还是直接 429。
	// 默认 false：配了代理池就意味着不想暴露本机 IP，悄悄直连会把这个前提废掉。
	FallbackDirect bool `json:"fallback_direct"`
	// FallbackAnon 决定 cookie 失效时是降级成匿名继续跑还是直接报错。
	// 默认 false：匿名档拿不到 3.1 Pro / 扩展思考 / 生图，降级了客户端也看不出来。
	FallbackAnon bool `json:"fallback_anon"`
	// GeminiBLAuto 决定是否定期从 /app 页面抓最新的 bl 版本号覆盖上面钉死的值。
	GeminiBLAuto bool `json:"gemini_bl_auto"`
	// MaxPromptBytes 是单次请求 prompt 的 UTF-8 字节上限，超了直接报错。
	// 单位是字节不是 token：实测上游的墙按字节走，跟语言无关，见 messages.go。
	// 0 = 关掉检查（原样发出，由上游从尾部静默截断）。
	MaxPromptBytes int `json:"max_prompt_bytes"`
	// MultiTurn 开启后走 Gemini 原生 conversation_id 服务端多轮：按历史前缀识别续接，
	// 命中就只发最新一句、历史留服务端，绕开单请求字节墙。匿名/登录都行，见 conversation.go。
	// 默认 false（保持现有"每轮拼全量 prompt"行为）。实测多轮不放大上下文窗口，只让长
	// 对话不撞单请求墙——对 Codex 这类长会话有用，对"喂长文档"没用。
	MultiTurn bool `json:"multi_turn"`
	// 出完结果自动删掉 gemini.google.com 上的这条会话（#19）。只登录态生效。默认 false。
	AutoDeleteConversation bool `json:"auto_delete_conversation"`
	// 匿名优先（#20）：不需要登录态能力的请求（纯文本、非思考、无工具、无图）走匿名、
	// 不占 cookie 账号，省账号额度；需要登录才挑号。默认 false，见 modelNeedsLogin。
	AnonFirst bool `json:"anon_first"`
	// 临时对话（#34）：塞 inner[45]=1，走网页端原生「临时对话」——不进会话列表、不入训练，
	// 取代 auto_delete_conversation（省掉删除 RPC）。只登录态有意义，与 multi_turn 冲突
	// （multi_turn 开着时不发）。默认 false，见 temporaryChatEligible。
	TemporaryChat bool `json:"temporary_chat"`
	// StrictModel 开启后拒绝已确认的模型不匹配；流式已发送的内容无法撤回。
	StrictModel bool `json:"strict_model"`
	// 只在跨额度带时改变选号优先级，同带保留健康度与轮转顺序。
	QuotaAwarePick bool `json:"quota_aware_pick"`
	// Cookie 池可见时的额度轮询间隔，单位秒。
	QuotaPollIntervalSec int `json:"quota_poll_interval_sec"`
}

const RuntimeConfigKey = "runtime_config"

var (
	rtMu  sync.RWMutex
	rtVal RuntimeConfig
)

// rtCfg 返回当前运行时配置的快照。
func Runtime() RuntimeConfig {
	rtMu.RLock()
	defer rtMu.RUnlock()
	return rtVal
}

// ApplyRuntime 原子发布配置快照；校验和持久化由调用方完成。
func ApplyRuntime(next RuntimeConfig) {
	rtMu.Lock()
	rtVal = next
	rtMu.Unlock()
}
