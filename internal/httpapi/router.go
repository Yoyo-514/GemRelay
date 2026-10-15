package httpapi

import (
	"net/http"

	"gemini-web2api/internal/config"
)

// NewHandler 注册兼容接口、MCP 和管理路由，不启动监听或后台任务。
func NewHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", requireAPIKey(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "OPTIONS":
			handleOptions(w, r)
		case "GET":
			handleModels(w, r)
		default:
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		}
	}))
	mux.HandleFunc("/v1/chat/completions", requireAPIKey(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "OPTIONS":
			handleOptions(w, r)
		case "POST":
			handleChatCompletions(w, r)
		default:
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		}
	}))
	mux.HandleFunc("/v1/responses", requireAPIKey(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "OPTIONS":
			handleOptions(w, r)
		case "POST":
			handleResponses(w, r)
		default:
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		}
	}))
	// /v1/videos —— OpenAI(Sora) 形状的异步视频生成。POST 建任务，GET 轮询，GET .../content 下 MP4。
	mux.HandleFunc("/v1/videos", requireAPIKey(handleCreateVideo))
	mux.HandleFunc("/v1/videos/", requireAPIKey(handleVideoItem))
	// MCP over HTTP（Streamable HTTP）：跟 OpenAI 接口同进程同端口，暴露 web_search。
	// 用同一把 API key 鉴权，客户端配 Authorization: Bearer <key> 连这个 URL。
	mux.HandleFunc("/mcp", requireAPIKey(handleMCPHTTP))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			handleRoot(w, r)
		} else {
			writeJSON(w, 404, map[string]string{"error": "not found"})
		}
	})

	// ─── Admin UI + API ────────────────────────────────────────────────
	if config.Startup().AdminEnabled {
		mux.HandleFunc("/admin", handleAdminUI)
		mux.HandleFunc("/admin/", handleAdminUI)
		mux.HandleFunc("/admin/api/login", handleAdminLogin)
		mux.HandleFunc("/admin/api/logout", handleAdminLogout)
		mux.HandleFunc("/admin/api/stats", requireAuth(handleAdminStats))
		mux.HandleFunc("/admin/api/timeseries", requireAuth(handleAdminTimeseries))
		mux.HandleFunc("/admin/api/requests", requireAuth(handleAdminRequests))
		mux.HandleFunc("/admin/api/proxies", requireAuth(handleAdminProxies))
		mux.HandleFunc("/admin/api/proxies/", requireAuth(handleAdminProxyItem))
		mux.HandleFunc("/admin/api/apikey", requireAuth(handleAdminAPIKey))
		mux.HandleFunc("/admin/api/usage", requireAuth(handleAdminUsage))
		mux.HandleFunc("/admin/api/config", requireAuth(handleAdminConfig))
		mux.HandleFunc("/admin/api/cookies", requireAuth(HandleAdminCookies))
		mux.HandleFunc("/admin/api/cookies/", requireAuth(handleAdminCookieItem))
		mux.HandleFunc("/admin/api/test", requireAuth(handleAdminTest))
		mux.HandleFunc("/admin/api/quota", requireAuth(handleAdminQuota))
	}

	return mux
}
