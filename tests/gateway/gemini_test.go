package tests

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"gemrelay/internal/config"
	"gemrelay/internal/gateway"
	"gemrelay/internal/gemini"
	"gemrelay/internal/pool"
	"gemrelay/internal/storage"
	"gemrelay/tests/testutil"
)

// 确认每个模型名解析出的 hex id 和 header 值正确。
func TestModelHeader(t *testing.T) {
	testutil.WithPoolCookie(t) // 让 3.1 Pro 可选，见 TestProHiddenWithoutCookie

	cases := []struct{ name, wantHex string }{
		{"gemini-3.6-flash", "fbb127bbb056c959"},
		{"gemini-3.5-flash-lite", "cf41b0e0dd7d53e5"},
		{"gemini-3.1-pro", "9d8ca3786ebdfbea"},
		{"gemini-4-argon", "95a4c0abcba24364"},
	}
	for _, c := range cases {
		name, mc, err := gateway.ResolveModel(c.name)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if mc.HexID != c.wantHex {
			t.Errorf("%s: hex=%s want %s", c.name, mc.HexID, c.wantHex)
		}
		h := gemini.BuildGeminiHeaders("", "", mc.HexID)
		got := h["x-goog-ext-525001261-jspb"]
		want := `[1,null,null,null,"` + c.wantHex + `"]`
		if got != want {
			t.Errorf("%s: header=%s want %s", c.name, got, want)
		}
		t.Logf("%-32s -> mode=%d header=%s", name, mc.Mode, got)
	}

	// @think=N 是历史遗留的假参数，剥掉后忽略，不报错也不改变路由
	name, mc, err := gateway.ResolveModel("gemini-3.6-flash@think=2")
	if err != nil || name != "gemini-3.6-flash" || mc.HexID != "fbb127bbb056c959" {
		t.Errorf("@think 后缀应被忽略, got name=%s hex=%s err=%v", name, mc.HexID, err)
	}
	// 未知模型仍然报错，不静默回落
	if _, _, err := gateway.ResolveModel("no-such-model"); err == nil {
		t.Error("unknown model should error")
	}
	// 已移除的旧别名必须明确报错，不能悄悄回落到 3.6 Flash
	for _, gone := range []string{"gemini-3.5-flash", "gemini-3.5-flash-thinking",
		"gemini-3.5-flash-thinking-lite", "gemini-auto", "gemini-flash-lite"} {
		if _, _, err := gateway.ResolveModel(gone); err == nil {
			t.Errorf("已移除的别名 %s 应该报错", gone)
		}
	}
	// 守的是「不许冒出假模型」：thinking 版复用同样的 hex，所以看**去重后的 hex 数**，
	// 不是条目数。服务端清单（otAQ7b）里 3.6/3.5-lite/3.1-pro/3.8/4 Argon = 5 个。
	hexes := map[string]bool{}
	for _, m := range gemini.Models {
		hexes[m.HexID] = true
	}
	if len(hexes) != 5 {
		t.Errorf("只应存在 5 个真模型 hex, got %d", len(hexes))
	}
	// 已确认扩展思考协议的模型都应该有 thinking 版
	for _, base := range []string{"fbb127bbb056c959", "cf41b0e0dd7d53e5", "9d8ca3786ebdfbea", "56fdd199312815e2"} {
		found := false
		for _, m := range gemini.Models {
			if m.HexID == base && m.Thinking {
				found = true
			}
		}
		if !found {
			t.Errorf("hex %s 缺 thinking 版", base)
		}
	}
}

func TestArgonAvailability(t *testing.T) {
	cases := []struct {
		name  string
		level int
	}{
		{"gemini-4-argon", 1},
		{"gemini-4-argon-low", 1},
		{"gemini-4-argon-medium", 4},
		{"gemini-4-argon-high", 2},
	}
	for _, c := range cases {
		if _, ok := gateway.AvailableModels()[c.name]; ok {
			t.Errorf("无 cookie 时不该暴露 %s", c.name)
		}
		if _, _, err := gateway.ResolveModel(c.name); err == nil ||
			!strings.Contains(err.Error(), "cookie") {
			t.Errorf("无 cookie 时选 %s 应提示需要登录, got %v", c.name, err)
		}
	}

	testutil.WithPoolCookie(t)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, ok := gateway.AvailableModels()[c.name]; !ok {
				t.Errorf("配了 cookie 时应暴露 %s", c.name)
			}
			name, mc, err := gateway.ResolveModel(c.name)
			if err != nil {
				t.Fatal(err)
			}
			if name != c.name || mc.HexID != "95a4c0abcba24364" || mc.Mode != 8 || mc.ThinkingLevel != c.level {
				t.Errorf("Argon 模型映射不正确: name=%s config=%+v", name, mc)
			}
			if mc.Thinking || mc.Tool != 0 {
				t.Errorf("Argon 应使用明确的思考档位，不启用旧开关或媒体工具: %+v", mc)
			}
		})
	}
	if _, _, err := gateway.ResolveModel("gemini-4-argon-thinking"); err == nil {
		t.Error("Argon 使用 low/medium/high 名称，不提供含义不明确的 thinking 别名")
	}
	name, mc, err := gateway.ResolveModel("gemini-4-argon-high@think=99")
	if err != nil || name != "gemini-4-argon-high" || mc.ThinkingLevel != 2 {
		t.Errorf("旧 @think 后缀不能改变 Argon 档位: name=%s config=%+v err=%v", name, mc, err)
	}
}

// 检查共用构造器的序列化结果：Argon 对齐三档抓包，旧模型保留原有参数。
func TestModelRequestParameters(t *testing.T) {
	argonLevels := map[string]int{
		"gemini-4-argon":        1,
		"gemini-4-argon-low":    1,
		"gemini-4-argon-medium": 4,
		"gemini-4-argon-high":   2,
	}
	for name, mc := range gemini.Models {
		t.Run(name, func(t *testing.T) {
			level := 1
			if mc.Thinking {
				level = 2
			}
			slots, mode, headerFlag := 97, mc.Mode, 1
			capabilities := `[4,5,6,8]`
			if argonLevel, ok := argonLevels[name]; ok {
				level = argonLevel
				slots, mode, headerFlag = 99, 8, 2
				capabilities = `[4,5,6,8,16,19,20,4,5,6,8,16,19,20]`
			}
			wantPayload := make([]interface{}, slots)
			wantPayload[6] = []interface{}{0}
			wantPayload[30] = []interface{}{4}
			wantPayload[68] = 1
			wantPayload[79] = mode
			wantPayload[80] = level
			wantPayload[91] = 0
			wantPayload[96] = 0
			if level != 1 {
				wantPayload[96] = 1
			}
			if _, ok := argonLevels[name]; ok {
				wantPayload[6] = []interface{}{1}
				wantPayload[30] = []interface{}{4, 16, 19}
				wantPayload[49] = 0
				wantPayload[67] = 0
				wantPayload[68] = 2
				wantPayload[91] = 1
				wantPayload[98] = 1
			}
			gotJSON, err := json.Marshal(gemini.NewModelPayload(mc))
			if err != nil {
				t.Fatal(err)
			}
			wantJSON, err := json.Marshal(wantPayload)
			if err != nil {
				t.Fatal(err)
			}
			if string(gotJSON) != string(wantJSON) {
				t.Errorf("模型请求体=%s want %s", gotJSON, wantJSON)
			}
			wantHeader := fmt.Sprintf(
				`[1,null,null,null,"%s",null,null,0,%s,null,null,%d,null,null,%d,%d,"test-uuid"]`,
				mc.HexID, capabilities, headerFlag, mode, level)
			if got := gemini.BuildModelHeader(mc.HexID, mc.Mode, gemini.ModelThinkingValue(mc), "test-uuid"); got != wantHeader {
				t.Errorf("模型请求头=%s want %s", got, wantHeader)
			}
			wantMinimal := fmt.Sprintf(`[1,null,null,null,"%s"]`, mc.HexID)
			if got := gemini.BuildModelHeader(mc.HexID, mc.Mode, gemini.ModelThinkingValue(mc), ""); got != wantMinimal {
				t.Errorf("空 UUID 时应保留最小请求头: got %s want %s", got, wantMinimal)
			}
		})
	}
}

// 中档成功请求的脱敏数组，保留位置和类型，避免按构造器逻辑重复错误预期。
func TestArgonCapturedPayload(t *testing.T) {
	const captured = `[["你好",0,null,null,null,null,0],["zh-CN"],null,"REDACTED","REDACTED",null,[1],1,null,null,1,0,null,null,null,null,null,[[0]],0,null,null,null,null,null,null,null,null,1,null,null,[4,16,19],null,null,null,null,null,null,null,null,null,null,[1],null,null,null,null,null,null,null,0,null,null,null,0,null,null,null,null,null,"REDACTED",null,[],null,null,null,null,null,0,2,null,null,null,null,null,null,null,null,null,null,8,4,null,null,null,null,null,null,null,null,null,null,1,null,null,null,null,1,null,1]`
	var want, got []json.RawMessage
	if err := json.Unmarshal([]byte(captured), &want); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(gemini.NewModelPayload(gemini.Models["gemini-4-argon-medium"]))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("请求体槽位数=%d want %d", len(got), len(want))
	}
	// 只核对模型固定参数，不比较语言、提示词、会话标识和动态令牌。
	for _, slot := range []int{6, 30, 49, 56, 67, 68, 79, 80, 91, 96, 98} {
		if string(got[slot]) != string(want[slot]) {
			t.Errorf("请求体[%d]=%s，网页抓包为 %s", slot, got[slot], want[slot])
		}
	}
}

// 抓包里真实的"被拒绝"响应：只有结束帧，没有内容帧（216 字节）。
// 注意结束帧里带 BardErrorInfo[1096]，但正常响应的结束帧同样带这个码，
// 所以判据只能是"有没有内容帧"。
const rejectedRaw = ")]}'\n\n122\n" +
	`[["wrb.fr",null,null,null,null,[13,null,[["type.googleapis.com/assistant.boq.bard.application.BardErrorInfo",[1096]]]]]]` +
	"\n56\n" + `[["di",192],["af.httprm",191,"8196459853603899163",2]]` +
	"\n25\n" + `[["e",4,null,null,216]]` + "\n"

// 正常响应：有内容帧，[39] 是模型 hex id、[42] 是显示名，结束帧同样带 1096。
const okRaw = ")]}'\n\n900\n" +
	`[["wrb.fr",null,"[null,[\"c_x\",\"r_y\"],null,null,[[\"rc_z\",[\"banana\"],null,null,null,null,null,null,[2],\"en\"]],null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,\"fbb127bbb056c959\",null,null,\"3.6 Flash\",true]"]]` +
	"\n122\n" +
	`[["wrb.fr",null,null,null,null,[13,null,[["type.googleapis.com/assistant.boq.bard.application.BardErrorInfo",[1096]]]]]]` + "\n"

func TestEmptyFrameDetection(t *testing.T) {
	if got := gemini.ExtractResponseText(rejectedRaw); got != "" {
		t.Errorf("被拒响应应解析出空文本, got %q", got)
	}
	if got := gemini.ExtractResponseText(okRaw); got != "banana" {
		t.Errorf("正常响应应解析出 banana, got %q", got)
	}
}

// usage 必须跟 requests 表口径一致（都走 tiktoken），不能退回 chars/4。
func TestUsageUsesTokenizer(t *testing.T) {
	gateway.InitTokenizer()
	// 取一段 tiktoken 与 chars/4 结果明显不同的样本，否则断言区分不出新旧实现。
	prompt := "请详细解释注意力机制的计算过程，并说明多头注意力为什么有效。"
	text := "自注意力通过查询、键、值三组投影计算token之间的相关性权重。"

	if gateway.TokenizerReady() && gateway.CountTokens(prompt) == len(prompt)/4 {
		t.Skipf("样本无区分度（tiktoken 与 chars/4 同为 %d），换样本再测", gateway.CountTokens(prompt))
	}

	u := gateway.BuildUsage(prompt, text, false)
	if u["prompt_tokens"] != gateway.CountTokens(prompt) || u["completion_tokens"] != gateway.CountTokens(text) {
		t.Errorf("chat usage 没走 countTokens: %v", u)
	}
	if u["total_tokens"] != u["prompt_tokens"]+u["completion_tokens"] {
		t.Errorf("total 不等于两者之和: %v", u)
	}
	if gateway.TokenizerReady() && u["prompt_tokens"] == len(prompt)/4 {
		t.Errorf("prompt_tokens 落在 chars/4 上，仍是旧实现: %v", u)
	}

	r := gateway.BuildUsage(prompt, text, true)
	if r["input_tokens"] != gateway.CountTokens(prompt) || r["output_tokens"] != gateway.CountTokens(text) {
		t.Errorf("responses usage 没走 countTokens: %v", r)
	}
	if _, ok := r["prompt_tokens"]; ok {
		t.Error("responses API 不应出现 prompt_tokens 字段")
	}
}

func TestToolChoice(t *testing.T) {
	for _, choice := range []interface{}{nil, "auto", "garbage"} {
		prompt, _ := gateway.MessagesToPrompt(
			[]map[string]interface{}{{"role": "user", "content": "hi"}},
			[]map[string]interface{}{{"type": "function", "function": map[string]interface{}{"name": "get_weather"}}}, choice)
		if !strings.Contains(prompt, "Only use tool_call blocks when needed.") {
			t.Errorf("默认工具选择未保留按需规则: choice=%v prompt=%s", choice, prompt)
		}
	}
	tools := []map[string]interface{}{
		{"type": "function", "function": map[string]interface{}{"name": "get_weather"}},
		{"type": "function", "function": map[string]interface{}{"name": "send_email"}},
	}
	msgs := []map[string]interface{}{{"role": "user", "content": "hi"}}

	// none：工具定义完全不进 prompt
	if p := mustPrompt(t, msgs, tools, "none"); strings.Contains(p, "get_weather") {
		t.Errorf("tool_choice=none 不该注入工具定义: %s", p)
	}
	// required：必须出现强制措辞
	p := mustPrompt(t, msgs, tools, "required")
	if !strings.Contains(p, "MUST call one of the tools") {
		t.Errorf("tool_choice=required 缺强制指令: %s", p)
	}
	// 指定函数：只留该函数，且点名它
	p = mustPrompt(t, msgs, tools, map[string]interface{}{"type": "function",
		"function": map[string]interface{}{"name": "get_weather"}})
	if !strings.Contains(p, `MUST call the tool "get_weather"`) {
		t.Errorf("指定函数缺强制指令: %s", p)
	}
	if strings.Contains(p, "send_email") {
		t.Errorf("指定函数时不该带上其它工具: %s", p)
	}
	// auto：保持原来的宽松措辞
	if p := mustPrompt(t, msgs, tools, "auto"); !strings.Contains(p, "when needed") {
		t.Errorf("auto 应保持宽松措辞: %s", p)
	}
}

// 流式的核心不变量：所有增量拼起来必须精确等于最终文本，既不能丢也不能重复。
func TestDeltaTrackerAccumulates(t *testing.T) {
	// 上游每帧是累积全文，逐帧变长
	frames := []string{
		"Transformer",
		"Transformer 的核心",
		"Transformer 的核心是自注意力",
		"Transformer 的核心是自注意力机制。",
	}
	d := &gemini.DeltaTracker{}
	var got string
	for _, f := range frames {
		got += d.Push(f)
	}
	want := frames[len(frames)-1]
	if got != want {
		t.Errorf("增量拼接=%q want %q", got, want)
	}
	if d.Text() != want {
		t.Errorf("emitted=%q want %q", d.Text(), want)
	}

	// 重复帧不得再次发出
	if extra := d.Push(frames[len(frames)-1]); extra != "" {
		t.Errorf("重复帧不该产生增量, got %q", extra)
	}
	// 变短的帧（乱序到达）也不发
	if extra := d.Push("Transformer"); extra != "" {
		t.Errorf("更短的帧不该产生增量, got %q", extra)
	}
	// 前缀对不上时跳过，emitted 保持不变
	before := d.Text()
	if extra := d.Push("完全不同的一段文本，比原来的还要长很多很多很多很多"); extra != "" {
		t.Errorf("非前缀帧不该产生增量, got %q", extra)
	}
	if d.Text() != before {
		t.Errorf("非前缀帧不该改动 emitted")
	}

	// remainingText 负责补齐 tracker 跳过的尾巴
	full := want + "补充的尾巴"
	if r := gateway.RemainingOf(full, d.Text()); r != "补充的尾巴" {
		t.Errorf("remainingText=%q want %q", r, "补充的尾巴")
	}
	// 非真流式（Emitted 为空）时应返回全文
	if r := gateway.RemainingOf(full, ""); r != full {
		t.Errorf("Emitted 为空时应返回全文")
	}
	// 前缀对不上时不补发，避免重复
	if r := gateway.RemainingOf("另一段内容", d.Text()); r != "" {
		t.Errorf("前缀对不上时不该补发, got %q", r)
	}
}

// 服务端会静默降级（匿名请求 3.1 Pro 实际给 3.5 Flash-Lite），只看请求名发现
// 不了，所以要从响应帧里把它自报的模型名取出来单独记录。
func TestExtractUpstreamModel(t *testing.T) {
	if got := gemini.ExtractUpstreamModel(okRaw); got != "3.6 Flash" {
		t.Errorf("正常响应应取到 '3.6 Flash', got %q", got)
	}
	// 带"扩展"后缀的模型名也要能取全，不能在空格处截断
	downgraded := `[["wrb.fr",null,"[null,[\"c_x\",\"r_y\"],null,null,[[\"rc_z\",[\"hi\"]]],null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,\"fbb127bbb056c959\",null,null,\"3.6 Flash 扩展\",true]"]]`
	if got := gemini.ExtractUpstreamModel(downgraded); got != "3.6 Flash 扩展" {
		t.Errorf("应取到 '3.6 Flash 扩展', got %q", got)
	}
	// 高档原始响应中的模型元数据；不保留账号、会话标识或长令牌。
	argonHigh := fmt.Sprintf(`[["wrb.fr",null,"[null,\"%s\",null,null,\"4 Argon High\",true,8,2]"]]`, "95a4c0abcba24364")
	if got := gemini.ExtractUpstreamModel(argonHigh); got != "4 Argon High" {
		t.Errorf("应取到 '4 Argon High', got %q", got)
	}
	// 被拒的响应没有模型名，不能瞎猜
	if got := gemini.ExtractUpstreamModel(rejectedRaw); got != "" {
		t.Errorf("被拒响应应返回空, got %q", got)
	}
}

// 没配 cookie 时 3.1 Pro 必然被静默降级成 Flash-Lite，不如干脆不暴露：
// 让客户端在选型时就拿到明确错误，而不是拿到一个"成功但其实不是 Pro"的回复。
// （配了有效 cookie 时它是真能用的，见 availableModels 注释。）
func TestProHiddenWithoutCookie(t *testing.T) {
	// cookie 池此时是空的（TestMain 给的是全新临时库）
	if _, ok := gateway.AvailableModels()["gemini-3.1-pro"]; ok {
		t.Error("无 cookie 时不该暴露 3.1 Pro")
	}
	// inner[80]（扩展思考）只在登录态生效，匿名带上服务端静默忽略，
	// 所以 thinking 版跟 3.1 Pro 一样不能暴露
	for name, m := range gateway.AvailableModels() {
		if m.Thinking {
			t.Errorf("无 cookie 时不该暴露 thinking 版: %s", name)
		}
	}
	if len(gateway.AvailableModels()) != 2 {
		t.Errorf("无 cookie 时应只剩 2 个模型, got %d", len(gateway.AvailableModels()))
	}
	_, _, err := gateway.ResolveModel("gemini-3.1-pro")
	if err == nil {
		t.Fatal("无 cookie 时选 3.1 Pro 应该报错")
	}
	// 错误信息要说清为什么、以及怎么办，不能只是 unknown model
	if !strings.Contains(err.Error(), "cookie") ||
		!strings.Contains(err.Error(), "downgraded") ||
		!strings.Contains(err.Error(), "admin panel") {
		t.Errorf("错误信息没解释原因和解法: %v", err)
	}

	testutil.WithPoolCookie(t)
	if _, ok := gateway.AvailableModels()["gemini-3.1-pro"]; !ok {
		t.Error("配了 cookie 时应暴露 3.1 Pro")
	}
	if _, _, err := gateway.ResolveModel("gemini-3.1-pro"); err != nil {
		t.Errorf("配了 cookie 时不该报错: %v", err)
	}
	for _, n := range []string{"gemini-3.6-flash-thinking", "gemini-3.5-flash-lite-thinking",
		"gemini-3.1-pro-thinking"} {
		if _, ok := gateway.AvailableModels()[n]; !ok {
			t.Errorf("配了 cookie 时应暴露 %s", n)
		}
	}
	// 真正不存在的模型仍然是 unknown model，不能被 cookie 提示盖掉
	if _, _, err := gateway.ResolveModel("no-such-model"); err == nil ||
		!strings.Contains(err.Error(), "unknown model") {
		t.Errorf("未知模型应报 unknown model, got %v", err)
	}
}

// modelNeedsLogin 是「没 cookie 时排除哪些模型」和「#20 匿名优先要不要占号」
// 共用的单一判据，逐个模型钉死：纯文本非思考的 3.6/3.5-lite 匿名可用，其余全需登录。
func TestModelNeedsLogin(t *testing.T) {
	anon := map[string]bool{"gemini-3.6-flash": true, "gemini-3.5-flash-lite": true}
	for name, mc := range gemini.Models {
		want := !anon[name]
		if got := gemini.ModelNeedsLogin(mc); got != want {
			t.Errorf("%s: modelNeedsLogin=%v want %v", name, got, want)
		}
	}
	// 跟 availableModels 的排除口径必须一致：无 cookie 时暴露的正好是匿名可用那批
	for name := range gateway.AvailableModels() {
		if gemini.ModelNeedsLogin(gemini.Models[name]) {
			t.Errorf("%s 被 availableModels 暴露却判为需登录，两处判据不一致", name)
		}
	}
}

// cookieAcctView 的 health 派生（#18 面板标红）：fail_count>0 一律红（含被自动停用的死号），
// 手动停用且无失败=灰，没成功过=待检测，其余=正常。
func TestCookieHealthView(t *testing.T) {
	cases := []struct {
		a    pool.CookieAccount
		want string
	}{
		{pool.CookieAccount{Status: "enabled", FailCount: 0, LastOkAt: 100}, "ok"},
		{pool.CookieAccount{Status: "enabled", FailCount: 3, LastOkAt: 100}, "dead"},
		{pool.CookieAccount{Status: "disabled", FailCount: 5, LastOkAt: 100}, "dead"},
		{pool.CookieAccount{Status: "disabled", FailCount: 0}, "disabled"},
		{pool.CookieAccount{Status: "enabled", FailCount: 0, LastOkAt: 0}, "unknown"},
	}
	for _, c := range cases {
		if got := c.a.Health(); got != c.want {
			t.Errorf("status=%s fail=%d ok=%d: health=%v want %s",
				c.a.Status, c.a.FailCount, c.a.LastOkAt, got, c.want)
		}
	}
}

// pickCookieAccount 的 SELECT+UPDATE 必须原子：N 个号被 N 个并发请求挑，
// 每个都该拿到不同的号。没加锁时两个请求会 SELECT 到同一个"最久未用"的号、
// 双双用它（跑 -race 更容易暴露）。
func TestPickCookieConcurrentDistinct(t *testing.T) {
	const n = 8
	for i := 0; i < n; i++ {
		id, err := pool.AccountAdd("conc", fmt.Sprintf("SAPISID=d%d; SID=x", i), "")
		if err != nil {
			t.Fatalf("插号失败: %v", err)
		}
		t.Cleanup(func() { _ = pool.AccountDelete(id) })
	}

	got := make([]int64, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			if a, ok := pool.PickCookieAccount(); ok {
				got[idx] = a.ID
			}
		}(i)
	}
	close(start) // 让所有 goroutine 尽量同时冲挑号
	wg.Wait()

	seen := map[int64]int{}
	for _, id := range got {
		seen[id]++
	}
	for id, c := range seen {
		if c > 1 {
			t.Errorf("账号 #%d 被并发挑中 %d 次，挑号未串行化", id, c)
		}
	}
}

// 匿名优先（#20）的挑号判据：开关关时恒 false（保持旧行为）；开时只有纯文本、
// 非思考、无工具、不带附件的请求才走匿名，其余一律挑号。
func TestAnonFirstEligible(t *testing.T) {
	set := func(on bool) {
		{
			nextRuntime := config.Runtime()
			nextRuntime.AnonFirst = on
			config.ApplyRuntime(nextRuntime)
		}
	}
	defer set(false)

	plain := gemini.Models["gemini-3.6-flash"]
	pro := gemini.Models["gemini-3.1-pro"]
	img := gemini.Models["gemini-image"]
	think := gemini.Models["gemini-3.6-flash-thinking"]

	set(false)
	if gateway.AnonymousEligible(plain, false) {
		t.Error("开关关时应恒 false（池里有号就用号）")
	}

	set(true)
	if !gateway.AnonymousEligible(plain, false) {
		t.Error("开 + 纯文本无附件应走匿名")
	}
	if gateway.AnonymousEligible(plain, true) {
		t.Error("带附件必须挑号（匿名引用被上游 1100 拒）")
	}
	if gateway.AnonymousEligible(pro, false) {
		t.Error("3.1 Pro 需登录，不能走匿名")
	}
	for _, name := range []string{"gemini-4-argon", "gemini-4-argon-low", "gemini-4-argon-medium", "gemini-4-argon-high"} {
		if gateway.AnonymousEligible(gemini.Models[name], false) {
			t.Errorf("%s 需登录，不能走匿名", name)
		}
	}
	if gateway.AnonymousEligible(img, false) {
		t.Error("生图需登录，不能走匿名")
	}
	if gateway.AnonymousEligible(think, false) {
		t.Error("扩展思考需登录，不能走匿名")
	}
}

// 错误分类要按"看到之后该做什么"分，尤其得把"上游瞬时拒绝"和"出口被封"
// 分开——前者重试即可，后者必须换 IP，混在一起排查时判断不了。
func TestClassifyError(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"", ""},
		{"strict_model：请求 gemini-4-argon，但上游实际使用 3.5 Flash-Lite", "model_downgraded"},
		{"upstream returned no content frame (raw 381 bytes)", "upstream_rejected"},
		{"direct IP slot full: rpm limit reached (configure proxies to scale)", "rate_limited_local"},
		{"all proxy slots full: rph limit reached", "rate_limited_local"},
		{"upstream HTTP 302: <HTML><HEAD><meta ...", "blocked_sorry"},
		{"upstream HTTP 429: too many requests", "upstream_429"},
		{"upstream HTTP 500: internal", "upstream_http_error"},
		{"unknown model: gemini-9", "bad_request"},
		{"image input not supported: ...", "bad_request"},
		{"dial tcp 1.2.3.4:10000: connectex: connection refused", "network"},
		{"remote error: tls: handshake failure", "network"},
		{"dial tcp: strict_model：connection failed", "network"},
	} {
		if got := storage.ClassifyError(c.in); got != c.want {
			t.Errorf("classifyError(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// mustPrompt 是测试里的便捷包装：messagesToPrompt 现在会在 prompt 超长时返回错误。
func mustPrompt(t *testing.T, msgs []map[string]interface{}, tools []map[string]interface{},
	toolChoice interface{}) string {
	t.Helper()
	p, _ := gateway.MessagesToPrompt(msgs, tools, toolChoice)
	return p
}
