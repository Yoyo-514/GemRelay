package gemini

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"

	"gemrelay/internal/config"
	"gemrelay/internal/cookies"
	"gemrelay/internal/transport"
)

const (
	rotatePageURL = "https://accounts.google.com/RotateCookiesPage" +
		"?og_pid=658&rot=3&origin=https%3A%2F%2Fgemini.google.com&exp_id=0"
	rotatePostURL = "https://accounts.google.com/RotateCookies"
	// og_pid 是产品标识，Gemini 固定 658；它既作为上面页面的 query，也回显在页面里。
	rotateProductID = 658
	// 服务端没给出间隔时的兜底值。
	DefaultRotateInterval = 10 * time.Minute
	// 启动后尽快刷一次：导入时 cookie 可能已经快到期，干等 10 分钟会直接过期。
	FirstRotateDelay = 15 * time.Second
	// 哨兵 payload。前导零是故意的，见文件头。
	rotate1PSIDTSBody = `[000,"-0000000000000000000"]`
	// 打太勤会 429。Gemini-API / notebooklm-py 都用 60 秒地板。
	min1PSIDTSInterval = 60 * time.Second
)

// 刷新 1PSIDTS 只带这一对。多带实测会 401。
var rotate1PSIDTSCookies = []string{"__Secure-1PSID", "__Secure-1PSIDTS"}

// RotationLimiter 管理账号刷新尝试的节流状态；零值可用。
type RotationLimiter struct {
	mu     sync.Mutex
	lastAt map[int64]time.Time
}

var rotationLimiter RotationLimiter

// 页面里形如：init('4162200486104360679', 658.0, 0.0, 0.0, 600.0)
// 第一个参数是这个会话的标识，最后一个是下次轮转的间隔秒数。
var rotateInitRe = regexp.MustCompile(`init\('([^']{4,64})'\s*,\s*([0-9.]+)\s*,[^)]*?([0-9.]+)\s*\)`)

// TryRotate1PSIDTS 用哨兵 payload 换发 1PSIDTS。没 __Secure-1PSID 或距上次不足
// 60 秒就跳过（不算失败）。
func TryRotate1PSIDTS(id int64, cookie, proxyURL string) (string, []string, error) {
	if cookies.Value(cookie, "__Secure-1PSID") == "" {
		return cookie, nil, nil
	}
	if ok, _ := rotationLimiter.Allow(id, time.Now()); !ok {
		config.Logf("[rotate] 账号 #%d 跳过 1PSIDTS 刷新（距上次不足 %s，避免 429）", id, min1PSIDTSInterval)
		return cookie, nil, nil
	}
	c2, names, err := rotate1PSIDTS(cookie, proxyURL)
	rotationLimiter.Note(id, err, time.Now())
	if err != nil {
		return cookie, nil, err
	}
	return c2, names, nil
}

// Allow 返回账号当前是否允许刷新，以及剩余等待时间。
func (l *RotationLimiter) Allow(id int64, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	last := l.lastAt[id]
	if last.IsZero() {
		return true, 0
	}
	elapsed := now.Sub(last)
	if elapsed >= min1PSIDTSInterval {
		return true, 0
	}
	return false, min1PSIDTSInterval - elapsed
}

// Note 记录成功或已到达上游的失败，不将网络错误计入节流。
func (l *RotationLimiter) Note(id int64, err error, now time.Time) {
	// 成功、401/403、429 都记时间，避免紧接着再打。网络错误不记，允许立刻重试。
	if err != nil {
		msg := err.Error()
		if !strings.Contains(msg, "HTTP 401") &&
			!strings.Contains(msg, "HTTP 403") &&
			!strings.Contains(msg, "HTTP 429") {
			return
		}
	}
	l.mu.Lock()
	if l.lastAt == nil {
		l.lastAt = make(map[int64]time.Time)
	}
	l.lastAt[id] = now
	l.mu.Unlock()
}

// BuildCookieRotation 返回刷新请求的 Cookie 子集和原始 JSPB 请求体。
func BuildCookieRotation(cookie string) (string, string) {
	return cookies.Subset(cookie, rotate1PSIDTSCookies), rotate1PSIDTSBody
}

// rotate1PSIDTS POST 哨兵 payload，把响应里的 Set-Cookie 合并回完整 cookie 串。
func rotate1PSIDTS(cookie, proxyURL string) (string, []string, error) {
	subset, body := BuildCookieRotation(cookie)
	headers := map[string]string{
		"Accept":         "*/*",
		"Content-Type":   "application/json",
		"Origin":         "https://accounts.google.com",
		"Cookie":         subset,
		"Cache-Control":  "no-cache",
		"Pragma":         "no-cache",
		"Sec-Fetch-Dest": "empty",
		"Sec-Fetch-Mode": "cors",
		"Sec-Fetch-Site": "same-origin",
	}
	status, setCookie, respBody, err := rotatePost(rotatePostURL, headers, []byte(body), proxyURL)
	if err != nil {
		return cookie, nil, err
	}
	if status == 401 || status == 403 {
		return cookie, nil, fmt.Errorf("RotateCookies 1PSIDTS 返回 HTTP %d（Chrome 导出的 cookie 可能受设备绑定限制，建议用 Firefox 重新导出）: %s",
			status, Truncate(string(respBody), 120))
	}
	if status != 200 {
		return cookie, nil, fmt.Errorf("RotateCookies 1PSIDTS 返回 HTTP %d: %s", status, Truncate(string(respBody), 120))
	}
	merged := cookies.MergeSetCookie(cookie, setCookie)
	return merged, setCookieNames(setCookie), nil
}

// RotateSIDCC 走浏览器 iframe 那条：GET 轮转页拿会话 id，再 POST [658, id]。
func RotateSIDCC(cookie, proxyURL string) (string, time.Duration, []string, error) {
	id, interval, pageSet, err := fetchRotateParams(cookie, proxyURL)
	if err != nil {
		return cookie, 0, nil, err
	}
	cookie = cookies.MergeSetCookie(cookie, pageSet)

	body := fmt.Sprintf(`[%d,"%s"]`, rotateProductID, id)
	headers := rotatePostHeaders()
	headers["Cookie"] = cookie
	status, setCookie, respBody, err := rotatePost(rotatePostURL, headers, []byte(body), proxyURL)
	if err != nil {
		return cookie, 0, nil, err
	}
	if status != 200 {
		return cookie, 0, nil, fmt.Errorf("RotateCookies 返回 HTTP %d: %s", status, Truncate(string(respBody), 120))
	}
	cookie = cookies.MergeSetCookie(cookie, setCookie)
	names := setCookieNames(append(append([]string{}, pageSet...), setCookie...))
	return cookie, interval, names, nil
}

func UniqueKeepOrder(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// setCookieNames 把 Set-Cookie 头里的名字抽出来去重，只用于日志。
func setCookieNames(headers []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, h := range headers {
		name := h
		if i := strings.Index(name, "="); i > 0 {
			name = name[:i]
		}
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

// fetchRotateParams 抓 RotateCookiesPage，取会话标识、服务端指定的间隔，以及这一发
// 自己带回的 Set-Cookie。
func fetchRotateParams(cookie, proxyURL string) (string, time.Duration, []string, error) {
	status, setCookie, body, err := rotateGet(rotatePageURL, cookie, proxyURL)
	if err != nil {
		return "", 0, nil, err
	}
	if status != 200 {
		return "", 0, nil, fmt.Errorf("取轮转页返回 HTTP %d", status)
	}
	m := rotateInitRe.FindSubmatch(body)
	if m == nil {
		// 页面拿到了却没有 init(...)，最可能是 cookie 已失效跳到了登录页。
		return "", 0, nil, fmt.Errorf("轮转页里没有 init(...)（cookie 可能已失效）")
	}
	id := string(m[1])
	interval := DefaultRotateInterval
	if sec, e := strconv.ParseFloat(string(m[3]), 64); e == nil && sec >= 60 && sec <= 3600 {
		interval = time.Duration(sec) * time.Second
	}
	return id, interval, setCookie, nil
}

// rotateGet 取轮转页。它在浏览器里是个 iframe 导航，所以 sec-fetch 那组跟普通
// XHR 完全不同（dest=iframe / mode=navigate / site=same-site），别套用默认值。
func rotateGet(url, cookie, proxyURL string) (int, []string, []byte, error) {
	return rotateDo("GET", url, map[string]string{
		"Cookie":                    cookie,
		"Accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7",
		"Referer":                   "https://gemini.google.com/",
		"Sec-Fetch-Dest":            "iframe",
		"Sec-Fetch-Mode":            "navigate",
		"Sec-Fetch-Site":            "same-site",
		"Sec-Fetch-User":            "?1",
		"Upgrade-Insecure-Requests": "1",
		"Priority":                  "u=0, i",
	}, nil, proxyURL)
}

// rotatePostHeaders 是 POST /RotateCookies 那一发的完整头，调用方只补 Cookie。
func rotatePostHeaders() map[string]string {
	return map[string]string{
		"Accept":         "*/*",
		"Content-Type":   "application/json",
		"Origin":         "https://accounts.google.com",
		"Referer":        rotatePageURL,
		"Cache-Control":  "no-cache",
		"Pragma":         "no-cache",
		"Priority":       "u=1, i",
		"Sec-Fetch-Dest": "empty",
		"Sec-Fetch-Mode": "same-origin",
		"Sec-Fetch-Site": "same-origin",
	}
}

func rotatePost(url string, headers map[string]string, body []byte, proxyURL string) (
	int, []string, []byte, error) {
	return rotateDo("POST", url, headers, body, proxyURL)
}

// rotateDo 走跟正式请求同一个出口：保活从别的 IP 发，等于告诉上游这个会话在两处活动。
//
// 两条传输路径共用同一份 header。以前只有走代理那条填浏览器头，直连那条
// 连 User-Agent 都不发 —— 同一个账号在上游看来会因为走没走代理而呈现两种客户端。
func rotateDo(method, url string, headers map[string]string, body []byte, proxyURL string) (
	int, []string, []byte, error) {
	var rdr io.Reader
	if body != nil {
		rdr = strings.NewReader(string(body))
	}
	merged := map[string]string{
		"Accept-Language": "en-US,en;q=0.9",
	}
	for k, v := range headers {
		merged[k] = v
	}
	if proxyURL != "" {
		req, err := http.NewRequest(method, url, rdr)
		if err != nil {
			return 0, nil, nil, err
		}
		transport.ApplyBrowserHeaders(req.Header)
		for k, v := range merged {
			req.Header.Set(k, v)
		}
		resp, err := transport.ProxyClient(proxyURL).Do(req)
		if err != nil {
			return 0, nil, nil, err
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		return resp.StatusCode, resp.Header.Values("Set-Cookie"), b, err
	}
	req, err := fhttp.NewRequest(method, url, rdr)
	if err != nil {
		return 0, nil, nil, err
	}
	transport.ApplyBrowserHeaders(req.Header)
	for k, v := range merged {
		req.Header.Set(k, v)
	}
	resp, err := transport.TLSClient().Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header.Values("Set-Cookie"), b, err
}
