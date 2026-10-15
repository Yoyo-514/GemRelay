package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"gemrelay/internal/config"
	"gemrelay/internal/httpapi"
	"gemrelay/web/admin"
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
		{"/admin/styles/base.css", "text/css", ":root", 200},
		{"/admin/scripts/app.js", "application/javascript", "fetch", 200},
		{"/admin/scripts/pages/cookies/quota.js", "application/javascript", "api/quota?account_id=", 200},
		{"/admin/scripts/pages/settings/fields.js", "application/javascript", "strict_model", 200},
		{"/admin/scripts", "", "", 404},
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

func TestAdminAssetReferences(t *testing.T) {
	handler := router(t, true)
	body, err := admin.Files.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	refs := regexp.MustCompile(`(?:src|href)="([^"]+)"`).FindAllSubmatch(body, -1)
	if len(refs) < 3 {
		t.Fatal("页面资源引用缺失")
	}
	for _, ref := range refs {
		path := string(ref[1])
		if strings.HasPrefix(path, "/") || strings.Contains(path, ":") || strings.Contains(path, "..") {
			t.Errorf("管理资源应使用本地相对路径: %s", path)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", "/admin/"+path, nil))
		if w.Code != 200 || w.Body.Len() == 0 {
			t.Errorf("资源不可读: %s: %d", path, w.Code)
		}
	}
}

func TestRuntimeFieldsAreEditable(t *testing.T) {
	body, err := admin.Files.ReadFile("scripts/pages/settings/fields.js")
	if err != nil {
		t.Fatal(err)
	}
	fields := make(map[string]int)
	for _, match := range regexp.MustCompile(`k: '([^']+)'`).FindAllSubmatch(body, -1) {
		fields[string(match[1])]++
	}
	typeOf := reflect.TypeFor[config.RuntimeConfig]()
	for i := 0; i < typeOf.NumField(); i++ {
		name := typeOf.Field(i).Tag.Get("json")
		if fields[name] != 1 {
			t.Errorf("运行时配置 %q 应恰好对应一个可编辑字段，实际 %d", name, fields[name])
		}
		delete(fields, name)
	}
	if len(fields) != 0 {
		t.Errorf("前端包含后端不存在的运行时字段: %v", fields)
	}
}
