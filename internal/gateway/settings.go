package gateway

import (
	"encoding/json"
	"fmt"

	"gemrelay/internal/config"
	"gemrelay/internal/storage"
)

// InitRuntimeConfig 用启动配置做基线，再把面板改过的值盖上去。
// 必须在 DB 打开之后调用。
func InitRuntimeConfig() {
	base := config.RuntimeConfig{
		RetryAttempts:   config.Startup().RetryAttempts,
		RetryDelaySec:   config.Startup().RetryDelaySec,
		RequestTimeout:  config.Startup().RequestTimeout,
		DefaultModel:    config.Startup().DefaultModel,
		PerIPConcurrent: config.Startup().PerIPConcurrent,
		PerIPRPM:        config.Startup().PerIPRPM,
		PerIPRPH:        config.Startup().PerIPRPH,
		RetentionDays:   config.Startup().RetentionDays,
		LogRequests:     config.Startup().LogRequests,
		Impersonate:     config.Startup().Impersonate,
		GeminiBL:        config.Startup().GeminiBL,

		ProxyCooldownMin: config.Startup().ProxyCooldownMin,
		FallbackDirect:   config.Startup().FallbackDirect,
		FallbackAnon:     config.Startup().FallbackAnon,
		GeminiBLAuto:     config.Startup().GeminiBLAuto,
		MaxPromptBytes:   config.Startup().MaxPromptBytes,
		MultiTurn:        config.Startup().MultiTurn,

		AutoDeleteConversation: config.Startup().AutoDeleteConversation,
		AnonFirst:              config.Startup().AnonFirst,
		TemporaryChat:          config.Startup().TemporaryChat,
		StrictModel:            config.Startup().StrictModel,
		QuotaAwarePick:         config.Startup().QuotaAwarePick,
		QuotaPollIntervalSec:   config.Startup().QuotaPollIntervalSec,
	}
	if raw := storage.GetValue(config.RuntimeConfigKey); raw != "" {
		saved := base
		if err := json.Unmarshal([]byte(raw), &saved); err == nil {
			if err := validateRuntimeConfig(saved); err == nil {
				base = saved
			} else {
				config.Logf("[config] 忽略 kv 里不合法的运行时配置: %v", err)
			}
		}
	}
	config.ApplyRuntime(base)
}

// validateRuntimeConfig 校验面板传来的值。
//
// 这些数字直接决定重试次数、超时和限流额度，是外部输入进敏感落点：
// 0 或负数会让限流器永远拒绝或永远放行，超大值能把单个请求挂死几小时。
// 上界给得比任何合理用法都宽，只挡明显离谱的输入。
func validateRuntimeConfig(c config.RuntimeConfig) error {
	type rangeCheck struct {
		name     string
		v        int
		min, max int
	}
	for _, r := range []rangeCheck{
		{"retry_attempts", c.RetryAttempts, 1, 10},
		{"retry_delay_sec", c.RetryDelaySec, 0, 60},
		{"request_timeout_sec", c.RequestTimeout, 5, 600},
		{"per_ip_concurrent", c.PerIPConcurrent, 0, 1000},
		{"per_ip_rpm", c.PerIPRPM, 0, 10000},
		{"per_ip_rph", c.PerIPRPH, 0, 100000},
		{"retention_days", c.RetentionDays, 1, 3650},
		{"proxy_cooldown_min", c.ProxyCooldownMin, 0, 10080},
		{"max_prompt_bytes", c.MaxPromptBytes, 0, 10000000},
		{"quota_poll_interval_sec", c.QuotaPollIntervalSec, 10, 3600},
	} {
		if r.v < r.min || r.v > r.max {
			return fmt.Errorf("%s=%d 超出允许范围 [%d, %d]", r.name, r.v, r.min, r.max)
		}
	}
	if _, _, err := ResolveModel(c.DefaultModel); err != nil {
		return fmt.Errorf("default_model 不可用: %v", err)
	}
	if c.Impersonate == "" {
		return fmt.Errorf("impersonate 不能为空")
	}
	if c.GeminiBL == "" {
		return fmt.Errorf("gemini_bl 不能为空")
	}
	return nil
}

// SaveRuntimeConfig 校验并持久化，成功后立刻生效。
func SaveRuntimeConfig(next config.RuntimeConfig) error {
	if err := validateRuntimeConfig(next); err != nil {
		return err
	}
	b, err := json.Marshal(next)
	if err != nil {
		return err
	}
	if err := storage.SetValue(config.RuntimeConfigKey, string(b)); err != nil {
		return err
	}
	config.ApplyRuntime(next)
	config.Logf("[config] 运行时配置已更新")
	return nil
}
