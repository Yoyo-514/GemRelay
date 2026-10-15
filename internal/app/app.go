package app

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"gemrelay/internal/config"
	"gemrelay/internal/gateway"
	"gemrelay/internal/httpapi"
	"gemrelay/internal/pool"
	"gemrelay/internal/storage"
)

// maskKey shows "sk-gemrelay-XXXX...YYYY" for banner. Empty stays empty.
func maskKey(k string) string {
	if k == "" {
		return "(disabled)"
	}
	if len(k) <= 12 {
		return "****"
	}
	return k[:12] + "..." + k[len(k)-4:]
}

// Run 是进程入口，由 cmd/gemrelay/main.go 调用。
func Run() {
	port := flag.Int("port", 0, "listening port (default: 8083 or config.json)")
	configPath := flag.String("config", "", "path to config.json")
	cookieFile := flag.String("cookie-file", "", "path to cookie file")
	proxy := flag.String("proxy", "", "HTTP proxy fallback when proxy pool is empty")
	impersonate := flag.String("impersonate", "", "TLS profile (chrome_146, chrome_133, firefox_147, ...)")
	dbPath := flag.String("db", "", "SQLite path (default: ./data/gemrelay.db)")
	adminToken := flag.String("admin-token", "", "admin token (empty = no auth, only safe on 127.0.0.1)")
	apiKey := flag.String("api-key", "", "API key for /v1/* (locked). Empty = use kv-table key (auto-gen on first boot, mutable from admin UI)")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("GemRelay %s\n", config.Version)
		return
	}

	if err := config.Load(*configPath); err != nil {
		fmt.Fprintf(os.Stderr, "config load error: %v\n", err)
		os.Exit(1)
	}
	if *port > 0 {
		config.Startup().Port = *port
	}
	if *cookieFile != "" {
		config.Startup().CookieFile = *cookieFile
	}
	if *proxy != "" {
		config.Startup().Proxy = *proxy
	}
	if *impersonate != "" {
		config.Startup().Impersonate = *impersonate
	}
	if *dbPath != "" {
		config.Startup().DBPath = *dbPath
	}
	if *adminToken != "" {
		config.Startup().AdminToken = *adminToken
	}
	if envTok := os.Getenv("ADMIN_TOKEN"); envTok != "" && config.Startup().AdminToken == "" {
		config.Startup().AdminToken = envTok
	}

	// boot DB and load proxy pool
	storage.DB()
	gateway.InitRuntimeConfig() // 面板改过的运行时配置盖在启动配置之上
	pool.LoadProxies()
	pool.SeedProxiesFromConfig() // --proxy / 遗留静态代理并进代理池
	pool.SeedCookiesFromConfig() // --cookie-file / 遗留单 cookie 并进 cookie 池
	resolvedAPIKey := httpapi.InitAPIKey(*apiKey)
	gateway.InitTokenizer()
	startScheduler()

	handler := httpapi.NewHandler()

	addr := fmt.Sprintf("%s:%d", config.Startup().Host, config.Startup().Port)
	server := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // streaming responses can be long
	}

	cookieStatus := "none (anonymous)"
	if total, enabled := pool.AccountCount(); enabled > 0 {
		cookieStatus = fmt.Sprintf("%d/%d in pool", enabled, total)
	}
	proxyStatus := "none (direct)"
	if n := len(pool.ListProxies()); n > 0 {
		proxyStatus = fmt.Sprintf("%d in pool", n)
	}
	var modelNames []string
	for n := range gateway.AvailableModels() {
		modelNames = append(modelNames, n)
	}
	sort.Strings(modelNames)

	fmt.Printf("GemRelay v%s\n", config.Version)
	fmt.Printf("  Listening:   http://%s\n", addr)
	fmt.Printf("  Base URL:    http://localhost:%d/v1\n", config.Startup().Port)
	fmt.Printf("  API key:     %s%s\n", maskKey(resolvedAPIKey),
		map[bool]string{true: "  (locked by flag/env)", false: "  (mutable in admin UI)"}[httpapi.APIKeyLocked()])
	if config.Startup().AdminEnabled {
		authMode := "no auth (open)"
		if config.Startup().AdminToken != "" {
			authMode = "token auth"
		}
		fmt.Printf("  Admin UI:    http://localhost:%d/admin  (%s)\n", config.Startup().Port, authMode)
	}
	fmt.Printf("  DB:          %s\n", config.Startup().DBPath)
	fmt.Printf("  Models:      %v\n", modelNames)
	fmt.Printf("  Cookie:      %s\n", cookieStatus)
	fmt.Printf("  Proxy:       %s\n", proxyStatus)
	fmt.Printf("  Impersonate: %s\n", config.Runtime().Impersonate)
	tokInfo := "chars/4 (fallback)"
	if gateway.TokenizerReady() {
		tokInfo = "tiktoken cl100k_base"
	}
	fmt.Printf("  Tokenizer:   %s\n", tokInfo)
	fmt.Printf("  Per-IP 限流: 并发=%d / RPM=%d / RPH=%d\n",
		config.Runtime().PerIPConcurrent, config.Runtime().PerIPRPM, config.Runtime().PerIPRPH)
	fmt.Printf("  Retry:       %dx / %ds\n", config.Runtime().RetryAttempts, config.Runtime().RetryDelaySec)
	fmt.Println()
	warnEnvProxyIgnored()

	if err := server.ListenAndServe(); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

// warnEnvProxyIgnored 在设了代理环境变量、但代理池是空的时候提醒一句。
//
// Linux 上的惯例是 HTTP 客户端自动读 HTTP_PROXY / ALL_PROXY（Go 标准库有现成的
// http.ProxyFromEnvironment），我们用的是显式 http.ProxyURL，一个都不读。这是
// 有意的：宿主机上随手一个 export 会悄悄改变出口 IP，而面板仍显示直连，排查时
// 会被带偏。但"没生效且毫无反馈"同样难查 —— 有用户按 systemd drop-in 的常规做法
// 注入了这几个变量，折腾很久才发现要在面板配。所以不读归不读，得说一声。
func warnEnvProxyIgnored() {
	// 大小写两种写法都查，但只报一个名字：Windows 的环境变量大小写不敏感，
	// 全列出来会变成 "HTTPS_PROXY / https_proxy" 这种看着像两个变量的噪音。
	var set []string
	for _, k := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY"} {
		if os.Getenv(k) != "" || os.Getenv(strings.ToLower(k)) != "" {
			set = append(set, k)
		}
	}
	if len(set) == 0 || len(pool.ListProxies()) > 0 {
		return
	}
	fmt.Printf("  ⚠ 检测到代理环境变量 %s，但本程序**不读**它们，当前走直连。\n"+
		"    请在面板「代理池」添加，或用 --proxy 启动参数（它会在启动时导入代理池）。\n\n",
		strings.Join(set, " / "))
}
