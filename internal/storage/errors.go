package storage

import "strings"

// ClassifyError 将请求错误归类为可操作的统计标签。
func ClassifyError(errStr string) string {
	switch {
	case errStr == "":
		return ""
	case strings.HasPrefix(errStr, "strict_model："):
		return "model_downgraded"
	case strings.Contains(errStr, "no content frame"):
		// 上游返回 200 但一个内容帧都没有。实测是瞬时拒绝，隔几分钟自行恢复，
		// 跟频率/并发/累积次数都无关，重试即可。
		return "upstream_rejected"
	case strings.Contains(errStr, "slot full") || strings.Contains(errStr, "limit reached"):
		return "rate_limited_local"
	case strings.Contains(errStr, "upstream HTTP 302"), strings.Contains(errStr, "sorry"):
		return "blocked_sorry"
	case strings.Contains(errStr, "upstream HTTP 429"):
		return "upstream_429"
	case strings.Contains(errStr, "upstream HTTP"):
		return "upstream_http_error"
	case strings.Contains(errStr, "unknown model"), strings.Contains(errStr, "not supported"):
		return "bad_request"
	default:
		// 连不上代理 / DNS / TLS 握手失败之类
		return "network"
	}
}
