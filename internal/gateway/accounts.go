package gateway

import (
	"strings"
	"time"

	"gemrelay/internal/cookies"
	"gemrelay/internal/gemini"
	"gemrelay/internal/pool"
)

// CheckAccountCookie 判断一条 cookie 还有没有登录态。
//
// 判据是 /app 页面里有没有 SNlM0e：cookie 失效时 Gemini 不报错，只是把你当匿名
// 用户，纯文本请求照样 200 —— 所以不能拿"请求成功"当有效性判据。这个页面没有
// SNlM0e 就说明服务端没认这个登录态。
//
// 只抓页面，不发对话，不消耗生成配额。
func CheckAccountCookie(a pool.CookieAccount) pool.CookieCheck {
	t0 := time.Now()
	picked, ok, err := pool.AcquireSlot(a.ProxyID)
	if !ok {
		return pool.CookieCheck{Detail: "拿不到出口：" + err.Error()}
	}
	defer pool.ReleaseSlot(picked.ID)
	proxyURL := picked.URL
	name := picked.Name
	if name == "" {
		name = "直连"
	}

	// 先作废缓存，否则可能拿到几分钟前的旧结论，检测就没意义了
	gemini.InvalidateXSRF(a.Cookie)
	_, err = gemini.GetXSRF(a.Cookie, proxyURL)
	took := time.Since(t0).Milliseconds()
	if err != nil {
		pool.MarkAccountResult(a.ID, false, err.Error())
		return pool.CookieCheck{Detail: explainCookieFailure(err), ProxyName: name, TookMs: took}
	}
	pool.BindAccountProxy(a.ID, a.ProxyID, picked.ID)
	pool.MarkAccountResult(a.ID, true, "")
	detail := "登录态有效"
	if cookies.SAPISID(a.Cookie) == "" {
		detail += "，但缺 SAPISID（算不出授权头，部分接口会被当匿名）"
	}
	return pool.CookieCheck{OK: true, Detail: detail, ProxyName: name, TookMs: took}
}

// explainCookieFailure 把底层错误翻成运维看得懂的结论。
func explainCookieFailure(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "no SNlM0e"):
		return "cookie 已失效：页面能打开但没有登录态，请求会被当匿名处理"
	case strings.Contains(msg, "HTTP 302"):
		return "cookie 无效：被重定向到登录页"
	case strings.Contains(msg, "HTTP 429"), strings.Contains(msg, "sorry"):
		return "出口被上游限流，换个代理再试（不是 cookie 的问题）"
	default:
		return "检测失败：" + msg
	}
}
