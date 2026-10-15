package pool

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"gemini-web2api/internal/config"
)

// QuotaAccount 旁路读取账号，不改 last_used_at；指定账号时允许停用状态。
func QuotaAccount(id int64) (*CookieAccount, bool) {
	if id != 0 {
		a := AccountByID(id)
		return a, a != nil && strings.TrimSpace(a.Cookie) != ""
	}
	items := AccountList()
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].FailCount != items[j].FailCount {
			return items[i].FailCount < items[j].FailCount
		}
		if items[i].LastUsedAt != items[j].LastUsedAt {
			return items[i].LastUsedAt < items[j].LastUsedAt
		}
		return items[i].ID < items[j].ID
	})
	for _, a := range items {
		if a.Status == "enabled" && strings.TrimSpace(a.Cookie) != "" {
			return &a, true
		}
	}
	return nil, false
}

// QuotaProxy 读取账号优先出口，不占生成 slot、不改变绑定或代理调度计数。
// 代理池非空却全部不可用时，仍遵守 fallback_direct，不能静默暴露本机 IP。
func QuotaProxy(preferID int64) (Proxy, error) {
	items := ListProxies()
	rt := config.Runtime()
	now := time.Now().Unix()
	var first *Proxy
	for _, p := range items {
		if !proxyUsable(p, now, rt.ProxyCooldownMin) {
			continue
		}
		if p.ID == preferID {
			return p, nil
		}
		if first == nil {
			copy := p
			first = &copy
		}
	}
	if first != nil {
		return *first, nil
	}
	if len(items) != 0 && !rt.FallbackDirect {
		return Proxy{}, fmt.Errorf("代理池没有可用出口，且未允许退回直连；请检查代理池后重试")
	}
	return Proxy{}, nil
}
