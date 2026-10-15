package tests

import (
	"errors"
	"testing"

	"gemrelay/internal/config"
	"gemrelay/internal/gateway"
	"gemrelay/internal/pool"
	"gemrelay/tests/testutil"
)

func setupAccountProxyTest(t *testing.T) {
	t.Helper()
	testutil.ResetState(t)
	old := config.Runtime()
	{
		nextRuntime := config.Runtime()
		nextRuntime.PerIPConcurrent = 1
		nextRuntime.PerIPRPM = 0
		nextRuntime.PerIPRPH = 0
		nextRuntime.ProxyCooldownMin = 0
		nextRuntime.FallbackDirect = false
		config.ApplyRuntime(nextRuntime)
	}
	t.Cleanup(func() {
		config.ApplyRuntime(old)
	})
}

func createAccountProxy(t *testing.T, name string) int64 {
	t.Helper()
	id, err := pool.ProxyCreate(name, "http://127.0.0.1:1", 1)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestBindAccountProxy(t *testing.T) {
	for _, state := range []string{"unbound", "healthy", "disabled", "deleted", "tripped", "stale"} {
		t.Run(state, func(t *testing.T) {
			setupAccountProxyTest(t)
			oldProxy := createAccountProxy(t, "original")
			selected := createAccountProxy(t, "selected")
			id, err := pool.AccountAdd("test", "SAPISID=dummy; SID=x", "")
			if err != nil {
				t.Fatal(err)
			}
			previous, want := oldProxy, selected
			if state == "unbound" {
				previous = 0
			} else {
				pool.BindAccountProxy(id, 0, oldProxy)
			}
			switch state {
			case "healthy":
				want = oldProxy
			case "disabled":
				enabled := false
				if err := pool.ProxyUpdate(oldProxy, "", "", &enabled, nil); err != nil {
					t.Fatal(err)
				}
			case "deleted":
				if err := pool.ProxyDelete(oldProxy); err != nil {
					t.Fatal(err)
				}
			case "tripped":
				for i := 0; i < 5; i++ {
					pool.RecordProxyResult(oldProxy, false, "unreachable")
				}
			case "stale":
				previous, want = 0, oldProxy
			}
			pool.BindAccountProxy(id, previous, selected)
			got := pool.AccountByID(id)
			if got == nil || got.ProxyID != want {
				t.Fatalf("账号绑定=%v want proxy %d", got, want)
			}
		})
	}
}

func TestRotateAccountDoesNotFallBackDirect(t *testing.T) {
	for _, state := range []string{"disabled", "tripped", "full"} {
		t.Run(state, func(t *testing.T) {
			setupAccountProxyTest(t)
			id := createAccountProxy(t, "unavailable")
			switch state {
			case "disabled":
				enabled := false
				if err := pool.ProxyUpdate(id, "", "", &enabled, nil); err != nil {
					t.Fatal(err)
				}
			case "tripped":
				for i := 0; i < 5; i++ {
					pool.RecordProxyResult(id, false, "unreachable")
				}
			case "full":
				if p, ok, err := pool.AcquireSlot(id); !ok || err != nil || p.ID != id {
					t.Fatalf("占用出口失败: proxy=%+v err=%v", p, err)
				}
				defer pool.ReleaseSlot(id)
			}
			// 必须在发任何网络请求之前失败，不能因未绑定而直连 Google。
			_, _, err := gateway.RotateAccount(pool.CookieAccount{})
			var limited *pool.RateLimitError
			if !errors.As(err, &limited) || limited.ProxyID != -1 {
				t.Fatalf("保活应返回出口不可用错误, got %v", err)
			}
		})
	}
}

func TestAccountProxySelection(t *testing.T) {
	setupAccountProxyTest(t)
	first := createAccountProxy(t, "first")
	second := createAccountProxy(t, "second")
	p, ok, err := pool.AcquireSlot(second)
	if !ok || err != nil || p.ID != second {
		t.Fatalf("应优先选择绑定出口: proxy=%+v ok=%v err=%v", p, ok, err)
	}
	pool.ReleaseSlot(p.ID)
	if occupied, ok, err := pool.AcquireSlot(second); !ok || err != nil || occupied.ID != second {
		t.Fatalf("占用绑定出口失败: proxy=%+v err=%v", occupied, err)
	}
	defer pool.ReleaseSlot(second)
	p, ok, err = pool.AcquireSlot(second)
	if !ok || err != nil || p.ID != first {
		t.Fatalf("绑定出口满时应选择其他代理，而非直连: proxy=%+v ok=%v err=%v", p, ok, err)
	}
	pool.ReleaseSlot(p.ID)
}
