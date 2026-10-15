package gateway

import (
	"gemrelay/internal/cookies"
	"gemrelay/internal/pool"
)

// loadCookie reads the cookie file (Netscape one-line format or JSON).
// loadCookie 返回 (cookie 串, SAPISID)。只有旁路探测在用 —— 正式请求那条路
// 在 streamGenerate 里自己挑号，因为它还要挨个换号和绑定出口。
//
// cookie 只有 cookie 池一个来源：挑一个 enabled 账号（最久未用优先，自动轮转
// 分散单 IP 上限）。池空 = 匿名。原来那条「池空回落单 cookie」的路径已经取消，
// 它的值在启动时被 seedCookiesFromConfig 并进池子了。
//
// 第三个返回值是池里那条记录的 ID，请求结束后拿它调 markCookieByStatus 回写健康度。
func loadCookie() (cookie, sapisid string) {
	if a, ok := pool.PickCookieAccount(); ok {
		return a.Cookie, cookies.SAPISID(a.Cookie)
	}
	return "", ""
}
