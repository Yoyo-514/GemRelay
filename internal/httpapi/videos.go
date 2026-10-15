package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"gemini-web2api/internal/gateway"
	"gemini-web2api/internal/gemini"
)

func handleCreateVideo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]any{"error": map[string]string{"message": "method not allowed"}})
		return
	}
	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]any{"error": map[string]string{"message": "bad json body"}})
		return
	}
	prompt, _ := req["prompt"].(string)
	if strings.TrimSpace(prompt) == "" {
		writeJSON(w, 400, map[string]any{"error": map[string]string{"message": "missing prompt", "type": "invalid_request_error"}})
		return
	}
	model, _ := req["model"].(string)
	if model == "" {
		model = "gemini-video"
	}
	// 校验模型确实是视频模型（查基础配置，不用 resolveModel —— 它无 cookie 时对登录态模型
	// 直接返「需要 cookie」错，那个错该留给 job 里报，别在这一步糊成「不是视频模型」）。
	if base, ok := gemini.Models[model]; !ok || base.Tool != gemini.ToolVideo {
		writeJSON(w, 400, map[string]any{"error": map[string]string{
			"message": "model must be a video model (gemini-video)", "type": "invalid_request_error"}})
		return
	}
	j := gateway.StartVideo(prompt, model)
	writeJSON(w, 200, j)
}

func handleVideoItem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, 405, map[string]any{"error": map[string]string{"message": "method not allowed"}})
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/v1/videos/")
	parts := strings.SplitN(rest, "/", 2)
	id := parts[0]
	j := gateway.VideoJobByID(id)
	if j == nil {
		writeJSON(w, 404, map[string]any{"error": map[string]string{"message": "video job not found", "type": "not_found"}})
		return
	}
	if len(parts) == 2 && parts[1] == "content" {
		status := j.Status
		mp4, mime := j.Content()
		if status != "completed" || len(mp4) == 0 {
			writeJSON(w, 404, map[string]any{"error": map[string]string{
				"message": "video not ready (status=" + status + ")", "type": "not_found"}})
			return
		}
		w.Header().Set("Content-Type", mime)
		w.WriteHeader(200)
		_, _ = w.Write(mp4)
		return
	}
	writeJSON(w, 200, j)
}
