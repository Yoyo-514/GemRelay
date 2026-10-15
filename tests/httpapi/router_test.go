package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gemini-web2api/internal/config"
	"gemini-web2api/internal/httpapi"
)

const apiKey = "sk-test-layout-key-for-router"

func router(t *testing.T, admin bool) http.Handler {
	t.Helper()
	old := *config.Startup()
	config.Startup().AdminEnabled = admin
	config.Startup().AdminToken = ""
	t.Cleanup(func() { *config.Startup() = old })
	httpapi.InitAPIKey(apiKey)
	return httpapi.NewHandler()
}

func TestRoutesRejectInvalidRequests(t *testing.T) {
	handler := router(t, true)
	for _, c := range []struct {
		method, path, body string
		status             int
	}{
		{"GET", "/missing", "", 404},
		{"POST", "/v1/models", "", 405},
		{"GET", "/v1/chat/completions", "", 405},
		{"POST", "/v1/chat/completions", "{", 400},
		{"POST", "/v1/responses", "{", 400},
		{"POST", "/v1/videos", "{", 400},
		{"POST", "/v1/videos", `{}`, 400},
		{"POST", "/v1/videos", `{"prompt":"hi","model":"not-a-model"}`, 400},
		{"GET", "/v1/videos/missing", "", 404},
		{"GET", "/mcp", "", 405},
		{"OPTIONS", "/v1/models", "", 204},
	} {
		t.Run(c.method+" "+c.path+" "+c.body, func(t *testing.T) {
			r := httptest.NewRequest(c.method, c.path, strings.NewReader(c.body))
			r.Header.Set("Authorization", "Bearer "+apiKey)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != c.status {
				t.Fatalf("status=%d want %d: %s", w.Code, c.status, w.Body.String())
			}
		})
	}
}

func TestRoutesRequireAPIKey(t *testing.T) {
	handler := router(t, false)
	for _, c := range []struct {
		header, value string
		status        int
	}{
		{"", "", 401}, {"Authorization", "Bearer incorrect", 401},
		{"Authorization", "Bearer " + apiKey, 200}, {"x-api-key", apiKey, 200},
	} {
		r := httptest.NewRequest("GET", "/v1/models", nil)
		if c.header != "" {
			r.Header.Set(c.header, c.value)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != c.status {
			t.Errorf("header=%q status=%d want %d", c.header, w.Code, c.status)
		}
		if c.status == 200 {
			var result struct {
				Object string                   `json:"object"`
				Data   []map[string]interface{} `json:"data"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.Object != "list" || len(result.Data) != 2 {
				t.Errorf("匿名模型列表不符合原约定: %+v", result)
			}
		}
	}
}

func TestEmbeddedAdminAssets(t *testing.T) {
	handler := router(t, true)
	for _, c := range []struct {
		path, contentType, contains string
		status                      int
	}{
		{"/admin", "", "", 301},
		{"/admin/", "text/html", "<!DOCTYPE html>", 200},
		{"/admin/chart.umd.min.js", "application/javascript", "Chart", 200},
		{"/admin/missing.js", "", "", 404},
	} {
		t.Run(c.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest("GET", c.path, nil))
			if w.Code != c.status {
				t.Fatalf("status=%d want %d", w.Code, c.status)
			}
			if c.contentType != "" && !strings.HasPrefix(w.Header().Get("Content-Type"), c.contentType) {
				t.Errorf("Content-Type=%q", w.Header().Get("Content-Type"))
			}
			if c.contains != "" && !strings.Contains(w.Body.String(), c.contains) {
				t.Errorf("资源缺少 %q", c.contains)
			}
			if c.status == 301 && w.Header().Get("Location") != "admin/" {
				t.Errorf("重定向必须保留反代子路径: %q", w.Header().Get("Location"))
			}
		})
	}
}

func TestAdminCanBeDisabled(t *testing.T) {
	w := httptest.NewRecorder()
	router(t, false).ServeHTTP(w, httptest.NewRequest("GET", "/admin/", nil))
	if w.Code != 404 {
		t.Fatalf("禁用管理端仍可访问: %d", w.Code)
	}
}
