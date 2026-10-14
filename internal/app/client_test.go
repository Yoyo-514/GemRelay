package app

import (
	"net/http"
	"strings"
	"testing"

	fhttp "github.com/bogdanfinn/fhttp"
)

func withBrowserProfile(t *testing.T, name string) {
	t.Helper()
	old := rtCfg()
	rtMu.Lock()
	rtVal.Impersonate = name
	rtMu.Unlock()
	t.Cleanup(func() {
		rtMu.Lock()
		rtVal = old
		rtMu.Unlock()
	})
}

func TestBrowserHeadersProfiles(t *testing.T) {
	cases := []struct {
		name, client, version, ua string
	}{
		{"chrome_120", "Chrome", "120", "Chrome/120.0.0.0"},
		{"chrome_124", "Chrome", "124", "Chrome/124.0.0.0"},
		{"chrome_131", "Chrome", "131", "Chrome/131.0.0.0"},
		{"chrome_133", "Chrome", "133", "Chrome/133.0.0.0"},
		{"chrome_144", "Chrome", "144", "Chrome/144.0.0.0"},
		{"chrome_146", "Chrome", "146", "Chrome/146.0.0.0"},
		{"chrome146", "Chrome", "146", "Chrome/146.0.0.0"},
		{"firefox_120", "Firefox", "120", "Firefox/120.0"},
		{"firefox_123", "Firefox", "123", "Firefox/123.0"},
		{"firefox_147", "Firefox", "147", "Firefox/147.0"},
		{"firefox147", "Firefox", "147", "Firefox/147.0"},
		{"firefox", "Firefox", "147", "Firefox/147.0"},
		{"zen", "Firefox", "147", "Firefox/147.0"},
		{"FIREFOX_147", "Firefox", "147", "Firefox/147.0"},
		{"safari_16_0", "Safari", "16.0", "Version/16.0 Safari/"},
		{"safari_ios_17_0", "iOS", "17.0", "Version/17.0 Mobile/"},
		// 未支持的名称继续回退默认 Chrome，不能仅凭 firefox 前缀发 Firefox 头。
		{"firefox_157", "Chrome", "146", "Chrome/146.0.0.0"},
		{"firefox_unknown", "Chrome", "146", "Chrome/146.0.0.0"},
		{"unknown", "Chrome", "146", "Chrome/146.0.0.0"},
		{"", "Chrome", "146", "Chrome/146.0.0.0"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			withBrowserProfile(t, c.name)
			id := resolveProfile(c.name).GetClientHelloId()
			if id.Client != c.client || id.Version != c.version {
				t.Fatalf("TLS 配置=%s/%s want %s/%s", id.Client, id.Version, c.client, c.version)
			}
			standard, err := http.NewRequest("GET", "https://gemini.google.com/app", nil)
			if err != nil {
				t.Fatal(err)
			}
			fingerprinted, err := fhttp.NewRequest("GET", "https://gemini.google.com/app", nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, header := range []map[string][]string{standard.Header, fingerprinted.Header} {
				h := http.Header(header)
				h.Set("Sec-CH-UA", "stale-chromium")
				h.Set("Sec-CH-UA-Mobile", "?0")
				h.Set("Sec-CH-UA-Platform", `"Windows"`)
				applyBrowserHeaders(header)
				if !strings.Contains(h.Get("User-Agent"), c.ua) {
					t.Errorf("User-Agent=%q want containing %q", h.Get("User-Agent"), c.ua)
				}
				if c.client == "Chrome" {
					if !strings.Contains(h.Get("Sec-CH-UA"), `"Chromium";v="`+c.version+`"`) ||
						h.Get("Sec-CH-UA-Mobile") != "?0" || h.Get("Sec-CH-UA-Platform") != `"Windows"` {
						t.Errorf("Chrome 客户端提示不匹配版本: %v", h)
					}
				} else {
					for _, key := range []string{"Sec-CH-UA", "Sec-CH-UA-Mobile", "Sec-CH-UA-Platform"} {
						if h.Get(key) != "" {
							t.Errorf("%s 不应携带 %s", c.client, key)
						}
					}
				}
				if h.Get("Sec-Fetch-Dest") != "empty" || h.Get("Sec-Fetch-Mode") != "cors" || h.Get("Sec-Fetch-Site") != "same-origin" {
					t.Errorf("缺少默认请求上下文: %v", h)
				}
			}
		})
	}
}

func TestBrowserHeadersKeepRequestContext(t *testing.T) {
	withBrowserProfile(t, "firefox")
	header := http.Header{
		"Cookie": {"SID=dummy"}, "Authorization": {"keep-auth"},
		"Accept": {"text/html"}, "Origin": {"https://accounts.google.com"},
		"Sec-Fetch-Dest": {"iframe"}, "Sec-Fetch-Mode": {"navigate"}, "Sec-Fetch-Site": {"cross-site"},
	}
	want := header.Clone()
	applyBrowserHeaders(header)
	for key := range want {
		if header.Get(key) != want.Get(key) {
			t.Errorf("调用方请求头 %s 被改写: got %q want %q", key, header.Get(key), want.Get(key))
		}
	}
	if !strings.Contains(header.Get("User-Agent"), "Firefox/147.0") {
		t.Errorf("浏览器标识未应用: %v", header)
	}
}
