package tests

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"gemini-web2api/internal/config"
	"gemini-web2api/internal/gateway"
	"gemini-web2api/internal/pool"
	"gemini-web2api/tests/testutil"
)

func TestQuotaRouteValidation(t *testing.T) {
	testutil.ResetState(t)
	handler := router(t, true)
	for _, c := range []struct {
		method, query string
		status        int
	}{
		{"POST", "", 405}, {"GET", "?account_id=0", 400},
		{"GET", "?account_id=-3", 400}, {"GET", "?account_id=abc", 400},
		{"GET", "?account_id=1.5", 400}, {"GET", "?account_id=9223372036854775808", 400},
		{"GET", "?account_id=1%3Bdrop", 400}, {"GET", "", 200},
		{"GET", "?account_id=%20%20", 200}, {"GET", "?account_id=%2042%20&refresh=1", 200},
	} {
		t.Run(c.method+c.query, func(t *testing.T) {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest(c.method, "/admin/api/quota"+c.query, nil))
			if w.Code != c.status {
				t.Fatalf("status=%d want %d: %s", w.Code, c.status, w.Body.String())
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("账号额度不能进入浏览器或代理共享缓存")
			}
			if c.status == 200 {
				var snap gateway.QuotaSnapshot
				if err := json.Unmarshal(w.Body.Bytes(), &snap); err != nil {
					t.Fatal(err)
				}
				if snap.OK || snap.Error == "" || snap.Cached {
					t.Fatalf("无账号应明确失败且不缓存: %+v", snap)
				}
				if c.query == "?account_id=%2042%20&refresh=1" && snap.AccountID != 42 {
					t.Fatal("失败也必须回填指定账号 ID")
				}
			}
		})
	}
}

func TestQuotaRequiresAdminAuth(t *testing.T) {
	handler := router(t, true)
	config.Startup().AdminToken = "quota-admin-token"
	for _, token := range []string{"", "wrong"} {
		r := httptest.NewRequest("GET", "/admin/api/quota?account_id=999999", nil)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatalf("未鉴权请求不应查询额度: %d", w.Code)
		}
	}
	r := httptest.NewRequest("GET", "/admin/api/quota?account_id=999999", nil)
	r.Header.Set("Authorization", "Bearer quota-admin-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("有效鉴权请求被拒绝: %d", w.Code)
	}
	w = httptest.NewRecorder()
	router(t, false).ServeHTTP(w, r)
	if w.Code != 404 {
		t.Fatal("管理端关闭时额度接口也必须关闭")
	}
}

func TestQuotaCannotBypassProxyPolicy(t *testing.T) {
	testutil.ResetState(t)
	old := config.Runtime()
	t.Cleanup(func() { config.ApplyRuntime(old) })
	rt := old
	rt.FallbackDirect = false
	config.ApplyRuntime(rt)
	id, err := pool.AccountAdd("private", "SAPISID=must-not-leak", "")
	if err != nil {
		t.Fatal(err)
	}
	proxyID, err := pool.ProxyCreate("disabled", "http://secret:password@quota.example:8080", 1)
	if err != nil {
		t.Fatal(err)
	}
	enabled := false
	if err := pool.ProxyUpdate(proxyID, "", "", &enabled, nil); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		snap := gateway.GetQuota(false, id)
		if snap.OK || snap.Error == "" || snap.Cached || snap.AccountID != id {
			t.Fatalf("禁止直连时应在出网前失败且不缓存: %+v", snap)
		}
	}
	if got := pool.AccountByID(id); got.LastUsedAt != 0 || got.FailCount != 0 || got.ProxyID != 0 {
		t.Fatalf("查询不应改写账号轮转、健康度或绑定: %+v", got)
	}
}
