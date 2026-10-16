package gateway

import (
	"time"

	"gemrelay/internal/config"
	"gemrelay/internal/gemini"
	"gemrelay/internal/pool"
)

// 额度按 20 个百分点分带；选号与管理查询共用 20 秒新鲜度。
const (
	quotaMinRemaining = 10
	quotaBandWidth    = 20
)

// 异步补查最多三路；名额占满后，由后续请求继续补查。
var quotaWarmSlots = make(chan struct{}, 3)

func quotaFresh(snap QuotaSnapshot, now time.Time) bool {
	if !snap.OK || now.Unix()-snap.FetchedAt >= int64(quotaCacheTTL/time.Second) {
		return false
	}
	for _, metric := range []*gemini.QuotaMetric{snap.FiveHour, snap.Weekly} {
		if metric != nil && metric.ResetAt > 0 && metric.ResetAt <= now.Unix() {
			return false
		}
	}
	return true
}

// SelectionPriority 返回选号分数：充足按带排序，未知居中，偏低时取剩余较多的账号。
// now 是本轮选号时间，20 秒内且尚待重置的快照参与计算。
func (snap QuotaSnapshot) SelectionPriority(now time.Time) int {
	if !quotaFresh(snap, now) {
		return 10
	}
	remaining, known := 100, false
	for _, metric := range []*gemini.QuotaMetric{snap.FiveHour, snap.Weekly} {
		if metric != nil {
			remaining, known = min(remaining, metric.RemainPct), true
		}
	}
	if !known {
		return 10
	}
	band := min(4, max(0, remaining)/quotaBandWidth)
	if remaining < quotaMinRemaining {
		return max(0, remaining)
	}
	return 20 + band
}

// peekQuota 从内存即时读取选号快照。
func peekQuota(key quotaKey, now time.Time) (QuotaSnapshot, bool) {
	quotaState.Lock()
	snap, ok := quotaState.cache[key]
	quotaState.Unlock()
	if !ok || !quotaFresh(snap, now) {
		return QuotaSnapshot{}, false
	}
	return snap, true
}

// PickCookieAccount 根据运行时配置选择账号，并跳过本次已经尝试的账号。
// pool 执行原子选号；多轮续接沿用原账号，新建会话时经过本入口。
func PickCookieAccount(skip map[int64]bool) (*pool.CookieAccount, bool) {
	if !config.Runtime().QuotaAwarePick {
		return pool.PickCookieAccountExcept(skip)
	}
	type pendingQuota struct {
		key quotaKey
		id  int64
	}
	var pending []pendingQuota
	now := time.Now()
	acct, ok := pool.PickCookieAccountByPriority(skip, func(a pool.CookieAccount) int {
		proxy, err := pool.QuotaProxy(a.ProxyID)
		if err != nil {
			return 10
		}
		key := accountQuotaKey(a, proxy.URL)
		if snap, fresh := peekQuota(key, now); fresh {
			return snap.SelectionPriority(now)
		}
		pending = append(pending, pendingQuota{key, a.ID})
		return 10
	})
	// 选号完成、锁和游标释放后，再启动补查。
	for _, p := range pending {
		warmQuota(p.key, p.id, now)
	}
	return acct, ok
}

func warmQuota(key quotaKey, accountID int64, now time.Time) {
	quotaState.Lock()
	if _, busy := quotaState.flights[key]; busy {
		quotaState.Unlock()
		return
	}
	if at, exists := quotaState.warmAt[key]; exists && now.Sub(at) < quotaCacheTTL {
		quotaState.Unlock()
		return
	}
	select {
	case quotaWarmSlots <- struct{}{}:
		quotaState.warmAt[key] = now
	default:
		quotaState.Unlock()
		return
	}
	quotaState.Unlock()
	go func() {
		defer func() { <-quotaWarmSlots }()
		GetQuota(false, accountID)
	}()
}
