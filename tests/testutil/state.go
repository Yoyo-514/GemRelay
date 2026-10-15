package testutil

import (
	"testing"

	"gemini-web2api/internal/config"
	"gemini-web2api/internal/pool"
	"gemini-web2api/internal/storage"
)

func ResetState(t *testing.T) {
	t.Helper()
	clean := func() {
		for _, a := range pool.AccountList() {
			_ = pool.AccountDelete(a.ID)
		}
		for _, p := range pool.ListProxies() {
			_ = pool.ProxyDelete(p.ID)
		}
		for _, k := range []string{
			"google_cookie", config.RuntimeConfigKey,
			"legacy_single_cookie_migrated", "seeded_cookie_id", "seeded_cookie_value",
			"legacy_static_proxy_migrated", "seeded_proxy_id", "seeded_proxy_url",
		} {
			_ = storage.SetValue(k, "")
		}
		config.Startup().CookieFile = ""
		config.Startup().Proxy = ""
	}
	clean()
	t.Cleanup(clean)
}

// WithPoolCookie 在当前测试期间放入一个账号，不访问真实上游。
func WithPoolCookie(t *testing.T) {
	t.Helper()
	id, err := pool.AccountAdd("test", "SAPISID=dummy; SID=x", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pool.AccountDelete(id) })
}
