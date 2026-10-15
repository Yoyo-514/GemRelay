package gateway

import (
	"crypto/sha256"
	"sync"
	"time"

	"gemrelay/internal/gemini"
	"gemrelay/internal/pool"
)

// QuotaSnapshot 是独立的账号额度快照，不包含 Cookie、代理 URL 或其他凭据。
type QuotaSnapshot struct {
	OK        bool                `json:"ok"`
	AccountID int64               `json:"account_id,omitempty"`
	Account   string              `json:"account,omitempty"`
	Proxy     string              `json:"proxy,omitempty"`
	Direct    bool                `json:"direct"`
	FiveHour  *gemini.QuotaMetric `json:"five_hour,omitempty"`
	Weekly    *gemini.QuotaMetric `json:"weekly,omitempty"`
	FetchedAt int64               `json:"fetched_at"`
	Cached    bool                `json:"cached,omitempty"`
	Error     string              `json:"error,omitempty"`
}

type quotaKey struct {
	accountID int64
	identity  [32]byte
}

type quotaEntry struct {
	snapshot QuotaSnapshot
	at       time.Time
}

type quotaFlight struct {
	done     chan struct{}
	snapshot QuotaSnapshot
}

const quotaCacheTTL = 20 * time.Second

var quotaState = struct {
	sync.Mutex
	cache   map[quotaKey]quotaEntry
	flights map[quotaKey]*quotaFlight
}{cache: make(map[quotaKey]quotaEntry), flights: make(map[quotaKey]*quotaFlight)}

// GetQuota 按实际账号和凭据/出口身份缓存，自动选号与指定同一账号共享并发请求。
// 仅合并正在执行的查询；失败不缓存，过期项在后续查询中清理。
func GetQuota(force bool, accountID int64) QuotaSnapshot {
	snap := QuotaSnapshot{AccountID: accountID, FetchedAt: time.Now().Unix()}
	acct, ok := pool.QuotaAccount(accountID)
	if !ok {
		snap.Error = "Cookie 池没有可查询的账号；请导入登录态或检查指定账号是否存在"
		return snap
	}
	snap.AccountID, snap.Account = acct.ID, acct.Label
	proxy, err := pool.QuotaProxy(acct.ProxyID)
	if err != nil {
		snap.Error = err.Error()
		return snap
	}
	snap.Proxy, snap.Direct = proxy.Name, proxy.ID == 0
	if snap.Direct {
		snap.Proxy = "直连"
	}
	key := quotaKey{accountID: acct.ID, identity: sha256.Sum256([]byte(acct.Cookie + "\x00" + proxy.URL))}
	now := time.Now()
	quotaState.Lock()
	for k, entry := range quotaState.cache {
		if now.Sub(entry.at) >= quotaCacheTTL {
			delete(quotaState.cache, k)
		}
	}
	if entry, exists := quotaState.cache[key]; exists && !force {
		quotaState.Unlock()
		result := quotaSnapshotAt(entry.snapshot, now)
		result.Account, result.Proxy = snap.Account, snap.Proxy
		result.Cached = true
		return result
	}
	if flight, exists := quotaState.flights[key]; exists {
		quotaState.Unlock()
		<-flight.done
		result := quotaSnapshotAt(flight.snapshot, time.Now())
		result.Account, result.Proxy = snap.Account, snap.Proxy
		return result
	}
	flight := &quotaFlight{done: make(chan struct{})}
	quotaState.flights[key] = flight
	quotaState.Unlock()

	snap.FiveHour, snap.Weekly, err = gemini.FetchQuota(acct.Cookie, proxy.URL)
	snap.FetchedAt = time.Now().Unix()
	if err != nil {
		snap.Error = err.Error()
	} else {
		snap.OK = true
	}
	quotaState.Lock()
	if snap.OK {
		quotaState.cache[key] = quotaEntry{snapshot: snap, at: time.Now()}
	} else {
		delete(quotaState.cache, key)
	}
	flight.snapshot = snap
	delete(quotaState.flights, key)
	close(flight.done)
	quotaState.Unlock()
	return quotaSnapshotAt(snap, time.Now())
}

// 深拷贝指标并按读取时间重算倒计时，不能返回共享缓存中的可变指针。
func quotaSnapshotAt(snap QuotaSnapshot, now time.Time) QuotaSnapshot {
	copyMetric := func(metric *gemini.QuotaMetric) *gemini.QuotaMetric {
		if metric == nil {
			return nil
		}
		copy := *metric
		copy.ResetInSec = max(int64(0), copy.ResetAt-now.Unix())
		return &copy
	}
	snap.FiveHour, snap.Weekly = copyMetric(snap.FiveHour), copyMetric(snap.Weekly)
	return snap
}
