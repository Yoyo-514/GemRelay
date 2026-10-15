package tests

import (
	"reflect"
	"testing"

	"gemrelay/internal/config"
	"gemrelay/internal/pool"
	"gemrelay/internal/storage"
	"gemrelay/tests/testutil"
)

func TestQuotaAccountIsReadOnly(t *testing.T) {
	testutil.ResetState(t)
	first, err := pool.AccountAdd("first", "SAPISID=first; SID=x", "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := pool.AccountAdd("second", "SAPISID=second; SID=x", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := storage.DB().Exec("UPDATE accounts SET last_used_at=100 WHERE id=?", first); err != nil {
		t.Fatal(err)
	}
	before := pool.AccountList()
	for range 2 {
		a, ok := pool.QuotaAccount(0)
		if !ok || a.ID != second {
			t.Fatalf("旁路选号不应推进轮转: %+v", a)
		}
	}
	if !reflect.DeepEqual(before, pool.AccountList()) {
		t.Fatal("额度查询修改了账号状态")
	}
	if err := pool.AccountSetStatus(second, "disabled"); err != nil {
		t.Fatal(err)
	}
	if a, ok := pool.QuotaAccount(second); !ok || a.ID != second {
		t.Fatal("指定账号应允许查询停用账号")
	}
	if a, ok := pool.QuotaAccount(0); !ok || a.ID != first {
		t.Fatal("自动选号不能选停用账号")
	}
	if _, ok := pool.QuotaAccount(second + 1000000); ok {
		t.Fatal("不存在的账号不能查到")
	}
}

func TestQuotaProxyPreservesSafetyAndCounters(t *testing.T) {
	testutil.ResetState(t)
	old := config.Runtime()
	t.Cleanup(func() { config.ApplyRuntime(old) })
	rt := old
	rt.FallbackDirect, rt.ProxyCooldownMin = false, 0
	config.ApplyRuntime(rt)
	if p, err := pool.QuotaProxy(0); err != nil || p.ID != 0 {
		t.Fatalf("无代理时应可直连: %+v %v", p, err)
	}
	id, err := pool.ProxyCreate("quota", "http://quota.example:8080", 1)
	if err != nil {
		t.Fatal(err)
	}
	other, err := pool.ProxyCreate("other", "http://other.example:8080", 1)
	if err != nil {
		t.Fatal(err)
	}
	before, slots := pool.ListProxies(), pool.AllSlotUsage()
	if p, err := pool.QuotaProxy(other); err != nil || p.ID != other {
		t.Fatalf("应优先账号绑定的出口: %+v %v", p, err)
	}
	if !reflect.DeepEqual(before, pool.ListProxies()) || !reflect.DeepEqual(slots, pool.AllSlotUsage()) {
		t.Fatal("额度出口选择不应修改代理或生成 slot 计数")
	}
	disabled := false
	if err := pool.ProxyUpdate(other, "", "", &disabled, nil); err != nil {
		t.Fatal(err)
	}
	if p, err := pool.QuotaProxy(other); err != nil || p.ID != id {
		t.Fatalf("绑定出口停用后应选健康代理: %+v %v", p, err)
	}
	for range 5 {
		pool.RecordProxyResult(id, false, "HTTP 429")
	}
	if _, err := pool.QuotaProxy(other); err == nil {
		t.Fatal("代理全不可用且禁止回退时不能直连")
	}
	rt.FallbackDirect = true
	config.ApplyRuntime(rt)
	if p, err := pool.QuotaProxy(other); err != nil || p.ID != 0 {
		t.Fatalf("明确允许后才可退回直连: %+v %v", p, err)
	}
	rt.FallbackDirect, rt.ProxyCooldownMin = false, 1
	config.ApplyRuntime(rt)
	if _, err := storage.DB().Exec("UPDATE proxies SET last_used=1 WHERE id=?", id); err != nil {
		t.Fatal(err)
	}
	pool.LoadProxies()
	if p, err := pool.QuotaProxy(id); err != nil || p.ID != id {
		t.Fatalf("冷却结束的代理应恢复可查询: %+v %v", p, err)
	}
}
