package gateway

import (
	"fmt"
	"strings"
	"time"

	"gemini-web2api/internal/config"
	"gemini-web2api/internal/gemini"
	"gemini-web2api/internal/pool"
)

// RotateAccount 给一个账号做一次保活：先刷 1PSIDTS，再刷 SIDCC。
// 返回服务端建议的下次间隔，以及这一轮实际刷新的 cookie 名。
func RotateAccount(a pool.CookieAccount) (time.Duration, []string, error) {
	p, ok, err := pool.AcquireSlot(a.ProxyID)
	if !ok {
		return 0, nil, fmt.Errorf("保活拿不到出口: %w", err)
	}
	defer pool.ReleaseSlot(p.ID)
	return rotateAccountOnProxy(a, p)
}

// rotateAccountOnProxy 复用调用方已占用的出口，避免请求内自愈再次占用限流名额。
func rotateAccountOnProxy(a pool.CookieAccount, p pool.Proxy) (time.Duration, []string, error) {
	proxyURL := p.URL

	cookie := a.Cookie
	var names []string
	var psidtsErr, sidccErr error
	interval := gemini.DefaultRotateInterval

	c2, n, err := gemini.TryRotate1PSIDTS(a.ID, cookie, proxyURL)
	if err != nil {
		psidtsErr = err
		config.Logf("[rotate] 账号 #%d 刷新 1PSIDTS 失败: %v", a.ID, err)
	} else {
		cookie = c2
		names = append(names, n...)
	}

	c3, iv, n2, err := gemini.RotateSIDCC(cookie, proxyURL)
	if err != nil {
		sidccErr = err
		config.Logf("[rotate] 账号 #%d SIDCC 保活失败: %v", a.ID, err)
	} else {
		cookie = c3
		names = append(names, n2...)
		if iv > 0 {
			interval = iv
		}
	}

	names = gemini.UniqueKeepOrder(names)
	if cookie != a.Cookie {
		old := a.Cookie
		pool.UpdateAccountCookie(a.ID, cookie)
		gemini.InvalidateXSRF(old)
	}
	if len(names) > 0 {
		config.Logf("[rotate] 账号 #%d 刷新了 %s", a.ID, strings.Join(names, ", "))
	}
	// 两条路都失败才算失败。1PSIDTS 刷到了但 iframe 页没 init，仍然是续命成功。
	if cookie == a.Cookie && len(names) == 0 {
		if psidtsErr != nil {
			return 0, nil, psidtsErr
		}
		if sidccErr != nil {
			return 0, nil, sidccErr
		}
	}
	pool.BindAccountProxy(a.ID, a.ProxyID, p.ID)
	return interval, names, nil
}

// RotateAllAccounts 给池子里每个启用的账号做一次保活，返回下次该等多久。
//
// 失败**不计入健康度**：保活打的是 accounts.google.com，跟对话能不能用是两码事，
// 网络抖一下就把号标成坏的，会让它在挑号时沉底，反而伤可用性。
func RotateAllAccounts() time.Duration {
	next := gemini.DefaultRotateInterval
	for _, a := range pool.AccountList() {
		if a.Status != "enabled" {
			continue
		}
		iv, _, err := RotateAccount(a)
		if err != nil {
			config.Logf("[rotate] 账号 #%d 保活失败: %v", a.ID, err)
			continue
		}
		if iv > 0 {
			next = iv
		}
	}
	return next
}
