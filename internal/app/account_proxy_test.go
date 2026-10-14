package app

import (
	"errors"
	"testing"
)

func setupAccountProxyTest(t *testing.T) {
	t.Helper()
	resetSeedState(t)
	old := rtCfg()
	rtMu.Lock()
	rtVal.PerIPConcurrent = 1
	rtVal.PerIPRPM = 0
	rtVal.PerIPRPH = 0
	rtVal.ProxyCooldownMin = 0
	rtVal.FallbackDirect = false
	rtMu.Unlock()
	t.Cleanup(func() {
		rtMu.Lock()
		rtVal = old
		rtMu.Unlock()
	})
}

func createAccountProxy(t *testing.T, name string) int64 {
	t.Helper()
	id, err := proxyCreate(name, "http://127.0.0.1:1", 1)
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
			id, err := accountAdd("test", "SAPISID=dummy; SID=x", "")
			if err != nil {
				t.Fatal(err)
			}
			previous, want := oldProxy, selected
			if state == "unbound" {
				previous = 0
			} else {
				bindAccountProxy(id, 0, oldProxy)
			}
			switch state {
			case "healthy":
				want = oldProxy
			case "disabled":
				enabled := false
				if err := proxyUpdate(oldProxy, "", "", &enabled, nil); err != nil {
					t.Fatal(err)
				}
			case "deleted":
				if err := proxyDelete(oldProxy); err != nil {
					t.Fatal(err)
				}
			case "tripped":
				for i := 0; i < proxyFailThreshold; i++ {
					recordProxyResult(oldProxy, false, "unreachable")
				}
			case "stale":
				previous, want = 0, oldProxy
			}
			bindAccountProxy(id, previous, selected)
			got := accountByID(id)
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
				if err := proxyUpdate(id, "", "", &enabled, nil); err != nil {
					t.Fatal(err)
				}
			case "tripped":
				for i := 0; i < proxyFailThreshold; i++ {
					recordProxyResult(id, false, "unreachable")
				}
			case "full":
				if ok, reason := trySlotAcquire(id); !ok {
					t.Fatalf("占用出口失败: %s", reason)
				}
				defer releaseSlot(id)
			}
			// 必须在发任何网络请求之前失败，不能因未绑定而直连 Google。
			_, _, err := rotateAccount(CookieAccount{})
			var limited *RateLimitError
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
	p, ok, err := acquireSlot(second)
	if !ok || err != nil || p.ID != second {
		t.Fatalf("应优先选择绑定出口: proxy=%+v ok=%v err=%v", p, ok, err)
	}
	releaseSlot(p.ID)
	if ok, reason := trySlotAcquire(second); !ok {
		t.Fatalf("占用绑定出口失败: %s", reason)
	}
	defer releaseSlot(second)
	p, ok, err = acquireSlot(second)
	if !ok || err != nil || p.ID != first {
		t.Fatalf("绑定出口满时应选择其他代理，而非直连: proxy=%+v ok=%v err=%v", p, ok, err)
	}
	releaseSlot(p.ID)
}
