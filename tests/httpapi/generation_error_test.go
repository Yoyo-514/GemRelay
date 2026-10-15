package tests

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"gemini-web2api/internal/gateway"
	"gemini-web2api/internal/httpapi"
)

func mismatchError() error {
	return fmt.Errorf("generation: %w", &gateway.ModelDowngradeError{
		Requested: "gemini-4-argon-high", Actual: "3.5 Flash-Lite", Expected: "4 argon",
	})
}

func TestModelFailureBeforeStream(t *testing.T) {
	w := httptest.NewRecorder()
	stream := httpapi.NewChatStream(w, "chat-test", 123, "gemini-4-argon-high")
	if stream.Started() {
		t.Fatal("创建流不应提前发送响应")
	}
	httpapi.WriteGenerationError(w, mismatchError())
	if w.Code != 429 || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("未开流应返回普通 429 JSON: %d %s", w.Code, w.Body.String())
	}
	var body struct {
		Error map[string]string `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error["type"] != "insufficient_quota" || body.Error["code"] != "model_downgraded" {
		t.Fatalf("模型错误分类丢失: %+v", body)
	}
}

func TestModelFailureAfterChatDelta(t *testing.T) {
	w := httptest.NewRecorder()
	stream := httpapi.NewChatStream(w, "chat-test", 123, "gemini-4-argon-high")
	stream.SendContent("already emitted")
	stream.Fail(mismatchError())
	body := w.Body.String()
	for _, want := range []string{"already emitted", `"type":"insufficient_quota"`, `"code":"model_downgraded"`, "data: [DONE]"} {
		if !strings.Contains(body, want) {
			t.Errorf("流式失败缺少 %q: %s", want, body)
		}
	}
	if w.Code != 200 || strings.Contains(body, `"finish_reason":"stop"`) {
		t.Fatal("已开流只能保留 HTTP 200，但不能发出成功结束信号")
	}
}

func TestModelFailureResponsesEvent(t *testing.T) {
	w := httptest.NewRecorder()
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(200)
	httpapi.WriteResponseFailure(w, "resp_test", "gemini-4-argon-high", mismatchError())
	body := w.Body.String()
	for _, want := range []string{"event: response.failed", `"status":"failed"`, `"type":"insufficient_quota"`, `"code":"model_downgraded"`, `"id":"resp_test"`} {
		if !strings.Contains(body, want) {
			t.Errorf("Responses 失败事件缺少 %q: %s", want, body)
		}
	}
	if strings.Contains(body, "response.completed") || strings.Contains(body, "[DONE]") {
		t.Fatal("Responses 失败不能伪装成功或混入 Chat 终态")
	}
}

func TestOrdinaryGenerationFailure(t *testing.T) {
	w := httptest.NewRecorder()
	httpapi.WriteGenerationError(w, fmt.Errorf("network failed"))
	if w.Code != 502 || !strings.Contains(w.Body.String(), `"type":"upstream_error"`) {
		t.Fatal("普通生成错误不应被误分类为模型额度不足")
	}
}
