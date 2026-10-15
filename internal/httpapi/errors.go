package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"gemrelay/internal/gateway"
)

func generationErrorBody(err error) map[string]string {
	body := map[string]string{"message": err.Error(), "type": "upstream_error"}
	var mismatch *gateway.ModelDowngradeError
	if errors.As(err, &mismatch) {
		body["type"] = "insufficient_quota"
		body["code"] = "model_downgraded"
	}
	return body
}

// WriteGenerationError 在响应尚未开始时写入生成错误，并保留模型不匹配的分类。
func WriteGenerationError(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway
	var mismatch *gateway.ModelDowngradeError
	if errors.As(err, &mismatch) {
		status = http.StatusTooManyRequests
	}
	writeJSON(w, status, map[string]interface{}{"error": generationErrorBody(err)})
}

// WriteResponseFailure 为已经打开的 Responses 流写失败终态，不发送成功终态。
func WriteResponseFailure(w http.ResponseWriter, id, model string, err error) {
	body, _ := json.Marshal(map[string]interface{}{
		"type": "response.failed",
		"response": map[string]interface{}{
			"id": id, "object": "response", "status": "failed", "model": model,
			"error": generationErrorBody(err),
		},
	})
	fmt.Fprintf(w, "event: response.failed\ndata: %s\n\n", body)
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}
