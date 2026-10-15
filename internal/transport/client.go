package transport

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"

	"gemini-web2api/internal/config"
)

var (
	clientCache   = map[string]tls_client.HttpClient{}
	clientCacheMu sync.RWMutex
)

// ResolveProfile maps a config string to a tls-client ClientProfile.
// Defaults to Chrome_146 (matches Python v2's curl_cffi chrome146).
func ResolveProfile(name string) profiles.ClientProfile {
	switch strings.ToLower(name) {
	case "chrome_120":
		return profiles.Chrome_120
	case "chrome_124":
		return profiles.Chrome_124
	case "chrome_131":
		return profiles.Chrome_131
	case "chrome_133":
		return profiles.Chrome_133
	case "chrome_144":
		return profiles.Chrome_144
	case "chrome_146", "chrome146":
		return profiles.Chrome_146
	case "firefox_120":
		return profiles.Firefox_120
	case "firefox_123":
		return profiles.Firefox_123
	case "firefox", "firefox_147", "firefox147", "zen":
		return profiles.Firefox_147
	case "safari_16_0":
		return profiles.Safari_16_0
	case "safari_ios_17_0":
		return profiles.Safari_IOS_17_0
	default:
		return profiles.Chrome_146
	}
}

// getTLSClient returns a tls-client (Chrome 146 fingerprint) for direct connections only.
// For proxied connections we use stdlib (see getStdlibClient) — tls-client's SOCKS5
// implementation is fragile compared to net/http.
func TLSClient() tls_client.HttpClient {
	clientCacheMu.RLock()
	if c, ok := clientCache[config.Runtime().Impersonate]; ok {
		clientCacheMu.RUnlock()
		return c
	}
	clientCacheMu.RUnlock()

	clientCacheMu.Lock()
	defer clientCacheMu.Unlock()
	if c, ok := clientCache[config.Runtime().Impersonate]; ok {
		return c
	}
	opts := []tls_client.HttpClientOption{
		tls_client.WithTimeoutSeconds(config.Runtime().RequestTimeout),
		tls_client.WithClientProfile(ResolveProfile(config.Runtime().Impersonate)),
		tls_client.WithNotFollowRedirects(),
	}
	client, err := tls_client.NewHttpClient(tls_client.NewNoopLogger(), opts...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[client] tls-client init failed: %v\n", err)
		os.Exit(1)
	}
	clientCache[config.Runtime().Impersonate] = client
	return client
}

var (
	stdlibClientCache sync.Map // proxyURL -> *http.Client
)

// getStdlibClient returns an http.Client routed through proxyURL.
// proxyURL must not be empty (caller checks).
func ProxyClient(proxyURL string) *http.Client {
	if cached, ok := stdlibClientCache.Load(proxyURL); ok {
		return cached.(*http.Client)
	}
	t := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  false,
		// 走代理时不强求 HTTP/2，部分代理不支持。
		ForceAttemptHTTP2: false,
	}
	if u, err := url.Parse(proxyURL); err == nil {
		t.Proxy = http.ProxyURL(u)
	}
	c := &http.Client{
		Timeout:   time.Duration(config.Runtime().RequestTimeout) * time.Second,
		Transport: t,
		// 跟 tls-client 一致：不自动跟随重定向（302 是诊断信号）
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	stdlibClientCache.Store(proxyURL, c)
	return c
}

// ChromeUA 是默认 Chrome 146 的应用层 UA 模板。
const ChromeUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36"

// ApplyBrowserHeaders 同时供 stdlib / fhttp 使用，浏览器家族和版本取自实际解析的 TLS 配置。
// 走代理仍使用 stdlib TLS；这里只统一应用层标识，不宣称能改变代理路径的 TLS 指纹。
func ApplyBrowserHeaders(headers map[string][]string) {
	h := http.Header(headers)
	id := ResolveProfile(config.Runtime().Impersonate).GetClientHelloId()
	for _, key := range []string{"Sec-CH-UA", "Sec-CH-UA-Mobile", "Sec-CH-UA-Platform"} {
		h.Del(key)
	}
	switch id.Client {
	case "Firefox":
		h.Set("User-Agent", fmt.Sprintf("Mozilla/5.0 (X11; Linux x86_64; rv:%s.0) Gecko/20100101 Firefox/%s.0", id.Version, id.Version))
	case "Safari":
		h.Set("User-Agent", fmt.Sprintf("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/%s Safari/605.1.15", id.Version))
	case "iOS":
		osVersion := strings.ReplaceAll(id.Version, ".", "_")
		h.Set("User-Agent", fmt.Sprintf("Mozilla/5.0 (iPhone; CPU iPhone OS %s like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/%s Mobile/15E148 Safari/604.1", osVersion, id.Version))
	default:
		h.Set("User-Agent", strings.Replace(ChromeUA, "Chrome/146.", "Chrome/"+id.Version+".", 1))
		h.Set("Sec-CH-UA", fmt.Sprintf(`"Chromium";v="%s", "Google Chrome";v="%s", "Not?A_Brand";v="24"`, id.Version, id.Version))
		h.Set("Sec-CH-UA-Mobile", "?0")
		h.Set("Sec-CH-UA-Platform", `"Windows"`)
	}
	// 页面导航、保活 iframe 等调用方已有的上下文头不能被默认值覆盖。
	for key, value := range map[string]string{
		"Sec-Fetch-Dest": "empty", "Sec-Fetch-Mode": "cors", "Sec-Fetch-Site": "same-origin",
	} {
		if h.Get(key) == "" {
			h.Set(key, value)
		}
	}
}
