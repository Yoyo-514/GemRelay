package pool

import (
	"sort"
	"sync"
	"time"

	"gemini-web2api/internal/config"
)

type ipSlot struct {
	mu        sync.Mutex
	inflight  int
	minuteWin []int64 // 滑动窗口里每个请求的 unix 时间戳（秒）
	hourWin   []int64
}

var (
	slotsMu sync.RWMutex
	slots   = map[int64]*ipSlot{} // proxy_id -> slot；0 = direct
)

func getSlot(proxyID int64) *ipSlot {
	slotsMu.RLock()
	s, ok := slots[proxyID]
	slotsMu.RUnlock()
	if ok {
		return s
	}
	slotsMu.Lock()
	defer slotsMu.Unlock()
	if s, ok := slots[proxyID]; ok {
		return s
	}
	s = &ipSlot{}
	slots[proxyID] = s
	return s
}

// trySlotAcquire 尝试占一个 slot；超额返回 false + 原因码。
// 调用方拿到 true 必须 deferred 调 slotRelease()。
//
//	"concurrent" — 已达瞬时并发上限
//	"rpm"        — 每分钟超限
//	"rph"        — 每小时超限
func trySlotAcquire(proxyID int64) (bool, string) {
	s := getSlot(proxyID)
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()
	// 清掉过期窗口
	cutMin := now - 60
	cutHour := now - 3600
	s.minuteWin = pruneTimestamps(s.minuteWin, cutMin)
	s.hourWin = pruneTimestamps(s.hourWin, cutHour)

	if config.Runtime().PerIPConcurrent > 0 && s.inflight >= config.Runtime().PerIPConcurrent {
		return false, "concurrent"
	}
	if config.Runtime().PerIPRPM > 0 && len(s.minuteWin) >= config.Runtime().PerIPRPM {
		return false, "rpm"
	}
	if config.Runtime().PerIPRPH > 0 && len(s.hourWin) >= config.Runtime().PerIPRPH {
		return false, "rph"
	}

	s.inflight++
	s.minuteWin = append(s.minuteWin, now)
	s.hourWin = append(s.hourWin, now)
	return true, ""
}

// slotRelease 释放并发计数（窗口计数自然过期，不在这里清）。
func slotRelease(proxyID int64) {
	s := getSlot(proxyID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inflight > 0 {
		s.inflight--
	}
}

// SlotUsage admin UI 看用量用。
type SlotUsage struct {
	ProxyID   int64 `json:"proxy_id"`
	Inflight  int   `json:"inflight"`
	RPM       int   `json:"rpm"`
	RPH       int   `json:"rph"`
	LimitConc int   `json:"limit_concurrent"`
	LimitRPM  int   `json:"limit_rpm"`
	LimitRPH  int   `json:"limit_rph"`
}

func slotUsage(proxyID int64) SlotUsage {
	s := getSlot(proxyID)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().Unix()
	s.minuteWin = pruneTimestamps(s.minuteWin, now-60)
	s.hourWin = pruneTimestamps(s.hourWin, now-3600)
	return SlotUsage{
		ProxyID:   proxyID,
		Inflight:  s.inflight,
		RPM:       len(s.minuteWin),
		RPH:       len(s.hourWin),
		LimitConc: config.Runtime().PerIPConcurrent,
		LimitRPM:  config.Runtime().PerIPRPM,
		LimitRPH:  config.Runtime().PerIPRPH,
	}
}

// AllSlotUsage 返回所有 slot 的用量快照（直连 + 全部代理）。
// AllSlotUsage 列出当前**可被调度到**的所有 IP slot，包括一次都还没用过的。
//
// 只返回 slots map 里已存在的是不够的：那个 map 是首次用到时才懒创建的，
// 服务刚重启时是空的，面板上「距离封禁红线」会整块空白——而那恰恰是部署时
// 最该盯的一屏。这里按 acquireSlot 的同一套规则枚举：配了代理就只列代理
// （代理存在时不会退回直连），否则列直连。
func AllSlotUsage() []SlotUsage {
	seen := map[int64]bool{}
	var ids []int64

	proxyMu.RLock()
	for _, p := range proxyCache {
		if p.Enabled {
			ids = append(ids, p.ID)
			seen[p.ID] = true
		}
	}
	hasProxies := len(ids) > 0
	proxyMu.RUnlock()

	if !hasProxies {
		ids = append(ids, 0)
		seen[0] = true
	}

	// 已有计数但已从池中移除/禁用的 slot 也带上，否则它的用量会凭空消失
	slotsMu.RLock()
	for id := range slots {
		if !seen[id] && (hasProxies == (id != 0)) {
			ids = append(ids, id)
		}
	}
	slotsMu.RUnlock()

	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := make([]SlotUsage, 0, len(ids))
	for _, id := range ids {
		out = append(out, slotUsage(id))
	}
	return out
}

func pruneTimestamps(ts []int64, cutoff int64) []int64 {
	// ts 是按时间递增追加的，找第一个 >= cutoff 的位置切掉前面
	i := 0
	for i < len(ts) && ts[i] < cutoff {
		i++
	}
	if i == 0 {
		return ts
	}
	return ts[i:]
}
