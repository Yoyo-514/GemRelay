package gateway

import (
	"fmt"

	"gemini-web2api/internal/config"
	"gemini-web2api/internal/gemini"
)

// ModelDowngradeError 保留模型不匹配的证据，供 HTTP 与流式错误统一分类。
type ModelDowngradeError struct {
	Requested string
	Actual    string
	Expected  string
}

func (e *ModelDowngradeError) Error() string {
	return fmt.Sprintf("strict_model：请求 %s，但上游实际使用 %s（期望 %s）。可能是账号额度耗尽或尚未开放该模型；请检查额度、换号或关闭严格模型校验。", e.Requested, e.Actual, e.Expected)
}

// StrictModelViolation 在生成结束后检查实际模型，不缓存或撤回已发送的流式内容。
func StrictModelViolation(model string, res *gemini.StreamResult) error {
	if !config.Runtime().StrictModel || res == nil {
		return nil
	}
	bad, want := gemini.ModelDowngraded(model, res.UpstreamModel)
	if !bad {
		return nil
	}
	return &ModelDowngradeError{Requested: model, Actual: res.UpstreamModel, Expected: want}
}
