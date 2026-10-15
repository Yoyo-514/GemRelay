package gateway

import (
	"fmt"
	"strings"
	"time"

	"gemini-web2api/internal/gemini"
	"gemini-web2api/internal/storage"
)

// BuildUsage 用 tiktoken 算 token 数，跟 requests 表里记的口径保持一致。
// responsesAPI=true 时用 /v1/responses 的字段名（input_tokens/output_tokens）。
func BuildUsage(prompt, text string, responsesAPI bool) map[string]int {
	return buildUsageWithReasoning(prompt, text, "", responsesAPI)
}

// buildUsageWithReasoning 在 usage 里单列思考链的 token 数。
//
// 思考链**不计入** completion_tokens：它是模型自己的推理过程，客户端默认不展示，
// 算进去等于让用户为看不见的输出买单（下游 newapi 是按 completion_tokens 计费的）。
// 单列在 completion_tokens_details.reasoning_tokens 里，跟 OpenAI 的做法一致，
// 想计费的人自己加。
func buildUsageWithReasoning(prompt, text, reasoning string, responsesAPI bool) map[string]int {
	// 媒体产物是 base64 data URL，上百万字符。不计进 output token：那是二进制内容，
	// 按 token 计费等于让用户为看不见的字节买单，下游 newapi 按 output token 收钱。
	in, out := CountTokens(prompt), CountTokens(gemini.StripDataURLs(text))
	if responsesAPI {
		return map[string]int{
			"input_tokens":  in,
			"output_tokens": out,
			"total_tokens":  in + out,
		}
	}
	u := map[string]int{
		"prompt_tokens":     in,
		"completion_tokens": out,
		"total_tokens":      in + out,
	}
	if reasoning != "" {
		u["reasoning_tokens"] = CountTokens(reasoning)
	}
	return u
}

func CallGemini(prompt, latest string, mc gemini.ModelConfig, tools []map[string]interface{},
	images []pendingUpload, onDelta, onReasoning func(string)) (string, []ToolCall, *gemini.StreamResult, error) {
	res, err := streamGenerateWithFiles(prompt, latest, mc, images, onDelta, onReasoning)
	if err != nil {
		// res 非 nil：失败时它只带归属（哪个号 / 哪个出口），给 recordRequest 用
		return "", nil, res, err
	}
	text := gemini.ExtractResponseText(res.Raw)
	// 画布：HTML 文档内联在响应里（不像图/乐要下载），从 immersive 结构里抠出来。
	// 标准文本提取只拿到 preamble（"I will generate…"），文档在 inner[4][0][30]…，
	// 用 extractCanvasDoc 单独取。
	if mc.Tool == gemini.ToolCanvas {
		doc := gemini.ExtractCanvasDoc(res.Raw)
		if doc == "" {
			return "", nil, res, fmt.Errorf("canvas generation failed: no HTML document in response (raw %d bytes)", len(res.Raw))
		}
		if text != "" && !strings.Contains(doc, text) {
			return text + "\n\n" + doc, nil, res, nil // preamble + 文档
		}
		return doc, nil, res, nil
	}
	// 媒体模型（生图/音乐）：生成 200 了但产物字节没取回来，直接报错而不是返回一个
	// 只有文字没有图的半成品 —— 客户端要的就是那张图/那段乐。
	if mc.Tool == gemini.ToolImage || mc.Tool == gemini.ToolMusic || mc.Tool == gemini.ToolVideo {
		if len(res.Artifacts) == 0 {
			msg := res.MediaErr
			if msg == "" {
				msg = "媒体产物取回失败"
			}
			return "", nil, res, fmt.Errorf("media generation succeeded but artifact retrieval failed: %s", msg)
		}
		// 产物以 base64 data URL 追加到正文（可能没正文，只有图/乐/视频）。
		text = gemini.AppendArtifactMarkdown(text, res.Artifacts)
		return text, nil, res, nil
	}
	if text == "" {
		// 上游拒绝时只回一个结束帧、没有内容帧（实测被拒时 raw 仅 216
		// 字节）。这种情况必须报错：以前会当成空回复返回 200 + content:null，
		// 客户端看不出请求其实失败了。
		// 注意不能用 BardErrorInfo 判错 —— 正常响应的结束帧里也带这个码。
		return "", nil, res, fmt.Errorf("upstream returned no content frame (raw %d bytes)", len(res.Raw))
	}
	var toolCalls []ToolCall
	if len(tools) > 0 {
		text, toolCalls = ParseToolCalls(text)
	}
	return text, toolCalls, res, nil
}

// RecordRequest writes one row of metadata to the requests table.
// Privacy: the prompt/response strings themselves are never persisted —
// only their length, model name, latency, status, and proxy info.
func RecordRequest(endpoint, model, prompt, response string, res *gemini.StreamResult, status int, errStr string, stream bool) {
	// 媒体产物是超长 base64，剥掉再算 token/长度：既不让它污染统计，也免得在请求线程里
	// 对上百万字符跑 tiktoken 白白拖慢。
	response = gemini.StripDataURLs(response)
	r := &storage.RequestRow{
		TS:            time.Now().Unix(),
		Model:         model,
		Status:        status,
		Error:         errStr,
		PromptChars:   len(prompt),
		ResponseChars: len(response),
		PromptTokens:  CountTokens(prompt),
		OutputTokens:  CountTokens(response),
		Endpoint:      endpoint,
	}
	if stream {
		r.Stream = 1
	}
	if res != nil {
		r.UpstreamModel = res.UpstreamModel
		r.TotalMs = res.TotalMs
		r.TTFBMs = &res.TTFBMs
		if res.ProxyID > 0 {
			r.ProxyID = &res.ProxyID
		}
		r.ProxyName = res.ProxyName
		if res.AccountID > 0 {
			r.AccountID = &res.AccountID
		}
		r.AccountLabel = res.AccountLabel
	}
	go storage.InsertRequest(r)
}

// UsageOf 是 buildUsageWithReasoning 的便捷包装，res 可能为 nil。
func UsageOf(prompt, text string, res *gemini.StreamResult) map[string]int {
	r := ""
	if res != nil {
		r = res.Reasoning
	}
	return buildUsageWithReasoning(prompt, text, r, false)
}

// RemainingOf 返回 full 里还没发出去的尾巴。emitted 为空时返回全文。
func RemainingOf(full, emitted string) string {
	if emitted == "" {
		return full
	}
	if strings.HasPrefix(full, emitted) {
		return full[len(emitted):]
	}
	// 前缀对不上（上游中途改写过），已发的收不回，不再补发以免重复。
	return ""
}
