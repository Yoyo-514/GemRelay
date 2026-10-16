package tests

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gemrelay/internal/config"
	"gemrelay/internal/cookies"
	"gemrelay/internal/gateway"
	"gemrelay/internal/gemini"
	"gemrelay/internal/pool"
	"gemrelay/internal/storage"
	"gemrelay/internal/transport"
	"gemrelay/tests/testutil"
)

func TestQuotaSelectionPriority(t *testing.T) {
	now := time.Now()
	metric := func(n int) *gemini.QuotaMetric {
		return &gemini.QuotaMetric{RemainPct: n, ResetAt: now.Add(time.Hour).Unix()}
	}
	for _, c := range []struct {
		name         string
		ok           bool
		five, weekly *gemini.QuotaMetric
		age          time.Duration
		want         int
	}{
		{"unknown", false, nil, nil, 0, 10},
		{"failed", false, metric(99), nil, 0, 10},
		{"missing metrics", true, nil, nil, 0, 10},
		{"tighter weekly", true, metric(99), metric(21), 0, 21},
		{"tighter five hour", true, metric(21), metric(99), 0, 21},
		{"five hour exhausted", true, metric(0), metric(99), 0, 0},
		{"weekly exhausted", true, metric(99), metric(0), 0, 0},
		{"threshold", true, metric(10), nil, 0, 20},
		{"below threshold", true, metric(9), nil, 0, 9},
		{"band edge", true, metric(20), nil, 0, 21},
		{"upper band", true, metric(99), nil, 0, 24},
		{"full", true, metric(100), nil, 0, 24},
		{"fresh snapshot", true, metric(99), nil, 19 * time.Second, 24},
		{"window boundary", true, metric(99), nil, 20 * time.Second, 10},
		{"old snapshot", true, metric(99), nil, 10 * time.Minute, 10},
		{"reset reached", true, &gemini.QuotaMetric{RemainPct: 0, ResetAt: now.Unix()}, nil, 0, 10},
	} {
		t.Run(c.name, func(t *testing.T) {
			snap := gateway.QuotaSnapshot{OK: c.ok, FiveHour: c.five, Weekly: c.weekly, FetchedAt: now.Add(-c.age).Unix()}
			if got := snap.SelectionPriority(now); got != c.want {
				t.Fatalf("priority=%d want %d", got, c.want)
			}
		})
	}
}

// 每个夹具使用独立代理客户端，在 HTTP 边界提供网页协议响应。
type quotaUpstream struct {
	remaining map[string]int
	resetAt   int64
	calls     atomic.Int32
	started   chan struct{}
	release   <-chan struct{}
}

func (u *quotaUpstream) RoundTrip(r *http.Request) (*http.Response, error) {
	body := `{"SNlM0e":"local-quota-token:1234567890"}`
	if r.URL.Path != "/app" {
		u.calls.Add(1)
		if u.started != nil {
			u.started <- struct{}{}
		}
		if u.release != nil {
			<-u.release
		}
		remaining := u.remaining[cookies.SAPISID(r.Header.Get("Cookie"))]
		payload := fmt.Sprintf(`[[[100,%g,1,[[%d,0]]]]]`, float64(100-remaining)/100, u.resetAt)
		encoded, _ := json.Marshal([]any{[]any{"wrb.fr", "jSf9Qc", payload}})
		body = string(encoded)
	}
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
}

func setupQuotaSelection(t *testing.T, values ...int) ([]int64, *quotaUpstream) {
	t.Helper()
	testutil.ResetState(t)
	old := config.Runtime()
	rt := old
	rt.QuotaAwarePick, rt.GeminiBLAuto = true, false
	rt.FallbackDirect = false
	rt.RequestTimeout = 5
	config.ApplyRuntime(rt)
	t.Cleanup(func() { config.ApplyRuntime(old) })
	proxyURL := "http://" + strings.ToLower(strings.ReplaceAll(t.Name(), "/", "-")) + ".example:8080"
	proxyID, err := pool.ProxyCreate("quota", proxyURL, 1)
	if err != nil {
		t.Fatal(err)
	}
	u := &quotaUpstream{remaining: map[string]int{}, resetAt: time.Now().Add(time.Hour).Unix()}
	client := transport.ProxyClient(proxyURL)
	original := client.Transport
	client.Transport = u
	t.Cleanup(func() { client.Transport = original })
	var ids []int64
	for i, remaining := range values {
		label := fmt.Sprintf("%s-%d", t.Name(), i)
		u.remaining[label] = remaining
		id, err := pool.AccountAdd(label, "SAPISID="+label+"; SID=x", "")
		if err != nil {
			t.Fatal(err)
		}
		pool.BindAccountProxy(id, 0, proxyID)
		ids = append(ids, id)
	}
	return ids, u
}

func primeQuotas(t *testing.T, ids []int64) {
	t.Helper()
	for _, id := range ids {
		if snap := gateway.GetQuota(true, id); !snap.OK {
			t.Fatalf("额度夹具查询失败: %+v", snap)
		}
	}
}

func TestQuotaAwareSelection(t *testing.T) {
	for _, c := range []struct {
		name   string
		a, b   int
		on     bool
		second bool
	}{
		{"cross band", 21, 99, true, true},
		{"same band", 50, 55, true, false},
		{"all low", 1, 9, true, true},
		{"threshold", 9, 10, true, true},
		{"off", 21, 99, false, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			ids, u := setupQuotaSelection(t, c.a, c.b)
			primeQuotas(t, ids)
			rt := config.Runtime()
			rt.QuotaAwarePick = c.on
			config.ApplyRuntime(rt)
			want, other := ids[0], ids[1]
			if c.second {
				want, other = other, want
			}
			got, ok := gateway.PickCookieAccount(nil)
			if !ok || got.ID != want {
				t.Fatalf("picked=%+v want %d", got, want)
			}
			if pool.AccountByID(want).LastUsedAt == 0 || pool.AccountByID(other).LastUsedAt != 0 {
				t.Fatal("使用时间应归属选中的账号")
			}
			got, ok = gateway.PickCookieAccount(map[int64]bool{want: true})
			if !ok || got.ID != other {
				t.Fatalf("重试应选择剩余候选: %+v", got)
			}
			if u.calls.Load() != 2 {
				t.Fatal("有效快照应直接参与选号")
			}
		})
	}
}

func TestQuotaSelectionHealthAndEligibility(t *testing.T) {
	ids, _ := setupQuotaSelection(t, 50, 55, 99)
	primeQuotas(t, ids)
	if _, err := storage.DB().Exec("UPDATE accounts SET fail_count=1 WHERE id=?", ids[1]); err != nil {
		t.Fatal(err)
	}
	if err := pool.AccountSetStatus(ids[2], "disabled"); err != nil {
		t.Fatal(err)
	}
	if got, ok := gateway.PickCookieAccount(nil); !ok || got.ID != ids[0] {
		t.Fatalf("同带应按健康度排序: %+v", got)
	}
	if got, ok := gateway.PickCookieAccount(map[int64]bool{ids[0]: true, ids[1]: true}); ok || got != nil {
		t.Fatalf("候选已用尽: %+v", got)
	}
}

func TestQuotaSelectionAtomic(t *testing.T) {
	const n = 12
	values := make([]int, n)
	for i := range values {
		values[i] = 55
	}
	ids, _ := setupQuotaSelection(t, values...)
	primeQuotas(t, ids)
	var wg sync.WaitGroup
	picked := make(chan int64, n)
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if a, ok := gateway.PickCookieAccount(nil); ok {
				picked <- a.ID
			}
		}()
	}
	wg.Wait()
	close(picked)
	seen := map[int64]bool{}
	for id := range picked {
		if seen[id] {
			t.Fatalf("并发选号重复: %d", id)
		}
		seen[id] = true
	}
	if len(seen) != n {
		t.Fatalf("已选账号=%d want %d", len(seen), n)
	}
}

func TestQuotaCacheIdentityAndCopy(t *testing.T) {
	ids, u := setupQuotaSelection(t, 99)
	first := gateway.GetQuota(false, ids[0])
	if !first.OK {
		t.Fatal(first.Error)
	}
	first.FiveHour.RemainPct = 1
	second := gateway.GetQuota(false, ids[0])
	if !second.Cached || second.FiveHour.RemainPct != 99 || u.calls.Load() != 1 {
		t.Fatalf("调用方应持有独立快照: %+v", second)
	}
	acct := pool.AccountByID(ids[0])
	pool.UpdateAccountCookie(acct.ID, acct.Cookie+"; __Secure-1PSIDTS=rotated")
	third := gateway.GetQuota(false, ids[0])
	if !third.OK || third.Cached || u.calls.Load() != 2 {
		t.Fatalf("凭据变化后应重新查询: %+v", third)
	}
}

func TestQuotaWarmIsAsyncAndBounded(t *testing.T) {
	ids, u := setupQuotaSelection(t, 55, 55, 55, 55, 55)
	release := make(chan struct{})
	u.release = release
	u.started = make(chan struct{}, 5)
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	selected := make(chan bool, 1)
	go func() { _, ok := gateway.PickCookieAccount(nil); selected <- ok }()
	select {
	case ok := <-selected:
		if !ok {
			t.Fatal("冷缓存应按原顺序选号")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("选号应即时返回")
	}
	for range 3 {
		select {
		case <-u.started:
		case <-time.After(2 * time.Second):
			t.Fatal("应启动三路补查")
		}
	}
	if _, ok := gateway.PickCookieAccount(nil); !ok {
		t.Fatal("补查期间应继续选号")
	}
	if u.calls.Load() != 3 {
		t.Fatalf("补查并发=%d want 3", u.calls.Load())
	}
	unblock()
	for _, id := range ids[:3] {
		if snap := gateway.GetQuota(false, id); !snap.OK {
			t.Fatal(snap.Error)
		}
	}
	if u.calls.Load() != 3 {
		t.Fatalf("管理查询与选号补查应合并: %d", u.calls.Load())
	}
}

func TestQuotaWarmRetryInterval(t *testing.T) {
	ids, u := setupQuotaSelection(t, 55)
	u.resetAt = time.Now().Add(-time.Second).Unix()
	primeQuotas(t, ids)
	release := make(chan struct{})
	u.release = release
	u.started = make(chan struct{}, 5)
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	if _, ok := gateway.PickCookieAccount(nil); !ok {
		t.Fatal("重置后的快照应按未知额度参与选号")
	}
	select {
	case <-u.started:
	case <-time.After(2 * time.Second):
		t.Fatal("到达重置时间后应启动补查")
	}
	unblock()
	if snap := gateway.GetQuota(false, ids[0]); !snap.OK {
		t.Fatal(snap.Error)
	}
	before := u.calls.Load()
	for range 20 {
		if _, ok := gateway.PickCookieAccount(nil); !ok {
			t.Fatal("补查间隔内应继续选号")
		}
	}
	if got := u.calls.Load(); got != before {
		t.Fatalf("20 秒内的选号补查应合并: calls=%d want %d", got, before)
	}
}

func TestQuotaRuntimeRoundTrip(t *testing.T) {
	testutil.ResetState(t)
	old, startup := config.Runtime(), *config.Startup()
	t.Cleanup(func() { config.ApplyRuntime(old); *config.Startup() = startup })
	*config.Startup() = config.Config{RetryAttempts: 3, RequestTimeout: 180, RetentionDays: 30, DefaultModel: "gemini-3.6-flash", Impersonate: "chrome_146", GeminiBL: "boq_gemini-web-uiserver_20261007.01_p0", QuotaAwarePick: true, QuotaPollIntervalSec: 60}
	gateway.InitRuntimeConfig()
	next := config.Runtime()
	next.QuotaAwarePick = true
	next.QuotaPollIntervalSec = 90
	if err := gateway.SaveRuntimeConfig(next); err != nil {
		t.Fatal(err)
	}
	config.ApplyRuntime(config.RuntimeConfig{})
	gateway.InitRuntimeConfig()
	if got := config.Runtime(); !got.QuotaAwarePick || got.QuotaPollIntervalSec != 90 {
		t.Fatalf("配置恢复: %+v", got)
	}
	for _, value := range []int{0, 9, 3601} {
		bad := next
		bad.QuotaPollIntervalSec = value
		if err := gateway.SaveRuntimeConfig(bad); err == nil {
			t.Fatalf("轮询间隔应符合范围: %d", value)
		}
	}
}
