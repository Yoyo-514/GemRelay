package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	"github.com/google/uuid"

	"gemini-web2api/internal/config"
	"gemini-web2api/internal/cookies"
	"gemini-web2api/internal/gemini"
	"gemini-web2api/internal/pool"
	"gemini-web2api/internal/transport"
)

// HasCookie 表示 cookie 池里有没有可用账号。决定 3.1 Pro 是否出现在模型列表里。
func HasCookie() bool {
	_, enabled := pool.AccountCount()
	return enabled > 0
}

// AnonymousEligible 判断「匿名优先」开关下这次请求能不能走匿名（不占 cookie 账号）：
// 开关开着 + 没带附件 + 模型不需要登录态。附件（图/视频）在对话里引用必须登录
// （匿名会被上游回 1100），所以带附件时一律挑号。开关关着永远返回 false（保持旧行为：
// 池里有号就用号）。
func AnonymousEligible(mc gemini.ModelConfig, hasAttachment bool) bool {
	return config.Runtime().AnonFirst && !hasAttachment && !gemini.ModelNeedsLogin(mc)
}

// TemporaryChatEligible 判断这次请求该不该走网页端原生「临时对话」（inner[45]=1，#34）。
//
// 与 multi_turn 冲突：临时对话不进会话列表，而 multi_turn 要按历史认出续接、靠服务端
// 留着这串对话——两个一起开，续接那半就没了。所以 multi_turn 开着时一律不发 inner[45]，
// 由 multi_turn 决定会话怎么留。调用点还会再要求有登录 cookie：匿名本来就不落账号，
// 发 inner[45] 是没验过的无谓偏离。
func TemporaryChatEligible() bool {
	return config.Runtime().TemporaryChat && !config.Runtime().MultiTurn
}

// AvailableModels 返回当前配置下值得暴露的模型。
//
// 没配 cookie 时排除 3.1 Pro：实测匿名请求它会被静默降级成 3.5 Flash-Lite，
// 客户端还以为自己用上了 Pro。与其让它"成功"，不如直接不提供、让选型时就报错。
//
// 配了有效 cookie 时它是真能用的：连打 6 次服务端回报的都是 "3.1 Pro" 本身，
// 且每次都带思考链（118-152 字符，普通 3.6 Flash 为 0）。早前记录的「免费号
// 登录也只能拿到 3.6 Flash 扩展」是在缺 XSRF token 的条件下测的，那时候带
// cookie 的请求根本发不出去（见 xsrf.go）。
func AvailableModels() map[string]gemini.ModelConfig {
	if HasCookie() {
		return gemini.Models
	}
	out := make(map[string]gemini.ModelConfig, len(gemini.Models))
	for k, v := range gemini.Models {
		if gemini.ModelNeedsLogin(v) {
			continue
		}
		out[k] = v
	}
	return out
}

// ResolveModel maps a model name to its config.
//
// "name@think=N" 后缀会被剥掉并忽略。旧版本把它写进 inner[17] 当思考深度，
// 那是误读：抓包显示 inner[17] 是会话内的轮次索引（首轮 [[0]]，带会话 id 的
// 第二轮 [[1]]，逐轮递增），跟思考深度无关。我们每次都开新会话，该值恒为 0。
// 后缀不报错只忽略，避免打断已经配了这个写法的客户端。
func ResolveModel(modelName string) (string, gemini.ModelConfig, error) {
	if idx := strings.Index(modelName, "@think="); idx >= 0 {
		modelName = modelName[:idx]
	}
	mc, ok := AvailableModels()[modelName]
	if !ok {
		if full, exists := gemini.Models[modelName]; exists && !HasCookie() {
			downgrade := "are silently downgraded to 3.5 Flash-Lite"
			if full.Tool > 0 {
				downgrade = "are silently downgraded to a plain \"Are you signed in?\" text reply"
			}
			return "", gemini.ModelConfig{}, fmt.Errorf(
				"%s is unavailable without a Google account cookie: anonymous requests for it "+
					"%s. Add a cookie in the admin panel (Cookie pool) or via --cookie-file to enable it",
				modelName, downgrade)
		}
		return "", gemini.ModelConfig{}, fmt.Errorf("unknown model: %s", modelName)
	}
	return modelName, mc, nil
}

// StreamGenerate POSTs to Gemini's StreamGenerate endpoint and returns raw body
// plus proxy/timing telemetry for the metrics layer.
// The 80-slot inner array is verbatim from the Python reference.
// onDelta 非 nil 时开启真流式：上游每写一帧就解析一次，跟已发出的内容做前缀
// diff，把新增部分立刻回调出去。上游每帧带的是累积全文而不是增量，diff 必须
// 自己做。一旦已经吐过内容就不再重试——重试会让客户端收到重复文本。
func StreamGenerate(prompt, latest string, mc gemini.ModelConfig,
	onDelta, onReasoning func(string)) (*gemini.StreamResult, error) {
	return streamGenerateWithFiles(prompt, latest, mc, nil, onDelta, onReasoning)
}

// streamGenerateWithFiles 同上，但可以带附件。
//
// 附件填 inner[0][3]，形状 [[[ref, 1], "文件名"], …]。附件只在登录态可用：
// 匿名能把文件传上去，但对话里一引用就被服务端回 1100。
func streamGenerateWithFiles(prompt, latest string, mc gemini.ModelConfig, pending []pendingUpload,
	onDelta, onReasoning func(string)) (*gemini.StreamResult, error) {
	var files []gemini.FileRef

	// 先挑号，再按它上次绑的出口挑代理 —— 同一个账号要尽量固定从同一个 IP 出去，
	// 否则一个号在几十个出口之间跳，在 Google 眼里就是账号共享的特征。
	//
	// 挑号排在 acquireSlot 之前不违反「取 XSRF 必须走正式出口」：挑号只读库、
	// 不发请求，真正发请求的是下面的 getXSRF，它在拿到 slot 之后。
	// #20 匿名优先：不需要登录态能力时不占用 cookie 账号，走匿名省额度（见 AnonymousEligible）。
	// 带附件（图/视频）时不能走匿名——下面 uploadBytes 那段会因 cookieStr=="" 直接报错。
	var acct *pool.CookieAccount
	if !AnonymousEligible(mc, len(pending) > 0) {
		if a, ok := pool.PickCookieAccount(); ok {
			acct = a
		}
	}
	preferProxy := int64(0)
	if acct != nil {
		preferProxy = acct.ProxyID
	}

	// 出错时也要把「用了哪个号 / 哪个出口」带回去，否则失败记录里全是空白。
	// picked 是拿到 slot 之后才填的，闭包捕获它，后面每次 attrib 都带上当时的出口。
	var picked pool.Proxy
	var cookieID int64
	var cookieLabel string
	attrib := func(err error) (*gemini.StreamResult, error) {
		return &gemini.StreamResult{
			AccountID: cookieID, AccountLabel: cookieLabel,
			ProxyID: picked.ID, ProxyName: picked.Name,
		}, err
	}

	// 通过限流器拿一个 slot（代理或直连）。所有 slot 满 → 直接 429。
	p, slotOK, slotErr := pool.AcquireSlot(preferProxy)
	if !slotOK {
		return attrib(slotErr)
	}
	picked = p
	defer pool.ReleaseSlot(picked.ID) // picked.ID=0 表示直连 slot

	// picked.URL 为空 = 直连 slot。代理只有代理池一个入口，没有别的兜底出口了
	// （原来那个「静态代理」字段已并进池子，见 seedProxiesFromConfig）。
	proxyURL := picked.URL
	pickedOK := picked.ID > 0 // 是否真用了代理池里的代理

	// endpoint 要等出口定下来才能拼：currentBL 可能顺手踢一次后台抓取，
	// 那个抓取必须跟正式请求走同一个出口，否则配了代理池也会从本机 IP 漏一次。
	reqid := time.Now().Unix() % 1000000
	endpoint := fmt.Sprintf(
		"https://gemini.google.com/_/BardChatUi/data/assistant.lamda.BardFrontendService/StreamGenerate?bl=%s&hl=en&_reqid=%d&rt=c",
		gemini.CurrentBL(proxyURL), reqid,
	)

	// 取 XSRF token。一个 cookie 失效不该让整个请求失败：当前号取不到就换下一个，
	// 最多试 maxCookieTries 个。不这么做的话，池子里 2 个号坏 1 个就会让大约一半
	// 请求挂掉，而每次失败看起来都像上游的问题，用户只会觉得"成功率莫名很低"。
	//
	// 换号**不换出口**：出口已经按第一个号的绑定选定了，同一个请求里再换出口没道理。
	cookieStr, sapisid, xsrfToken := "", "", ""
	var lastCookieErr error
	tried := map[int64]bool{}
	// 每个号只给一次「轮转后重试」的机会，避免在一个请求里反复打 accounts.google.com
	rotatedOnce := map[int64]bool{}
	for acct != nil && len(tried) < gemini.MaxCookieTries {
		tried[acct.ID] = true
		// 归属先记上：这一轮失败了也留痕，面板上看得出是哪个号在坏
		cookieID, cookieLabel = acct.ID, pool.AccountDisplayName(acct)
		tok, err := gemini.GetXSRF(acct.Cookie, proxyURL)
		if err == nil {
			cookieStr, sapisid, xsrfToken = acct.Cookie, cookies.SAPISID(acct.Cookie), tok
			// 只在「还没绑过」或「绑的出口已经没了」时写绑定。
			//
			// 绝不因为"这次走的是别的出口"就覆盖：出口是按**本次第一个挑中的号**
			// 的绑定选的，而挑号会换（新加的号 last_used_at=0 排在最前，撞上坏号
			// 就会换）。拿别人的出口覆盖当前号的绑定，等于每次撞上坏号就把好号的
			// 粘性打散一次 —— 实测就是这么散掉的。
			pool.BindAccountProxy(acct.ID, acct.ProxyID, picked.ID)
			break
		}
		// 取不到 SNlM0e 基本等于这个 cookie 已失效（页面把我们当匿名用户了）。
		// 换号之前先给它一次机会：强制轮转一次再重取。
		//
		// 轮转会换发 __Secure-1PSIDTS（约 30 分钟过期的那张票）并合并 *SIDCC。
		// 只是票过期、持久身份还在的号，这一步能救回来；救不回来再换号。
		// 只试一次，且只在这一轮。
		if !rotatedOnce[acct.ID] {
			rotatedOnce[acct.ID] = true
			if _, _, rerr := rotateAccountOnProxy(*acct, picked); rerr == nil {
				if fresh := pool.AccountByID(acct.ID); fresh != nil {
					if tok2, err2 := gemini.GetXSRF(fresh.Cookie, proxyURL); err2 == nil {
						config.Logf("[cookie] 账号 #%d 轮转后恢复可用", acct.ID)
						acct = fresh
						cookieStr, sapisid, xsrfToken = fresh.Cookie, cookies.SAPISID(fresh.Cookie), tok2
						pool.BindAccountProxy(fresh.ID, fresh.ProxyID, picked.ID)
						break
					}
				}
			}
		}
		// 救不回来：记一次失败让面板上看得出是哪个号该换了，然后换下一个。
		pool.MarkCookieByStatus(acct.ID, 401, err.Error())
		lastCookieErr = err
		config.Logf("[cookie] 账号 #%d 不可用，换下一个：%v", acct.ID, err)
		acct, _ = pool.PickCookieAccountExcept(tried) // 取不到时返回 nil，循环自然结束
	}
	if lastCookieErr != nil && cookieStr == "" {
		if !config.Runtime().FallbackAnon {
			// 默认报错而不是降级：cookie 失效后上游不会拒绝，只是把你当匿名用户，
			// 纯文本请求照样 200 —— 于是 3.1 Pro 被静默降级成 3.5 Flash-Lite、
			// 思考链消失，客户端完全看不出来。宁可明确失败也不给假的成功。
			return attrib(fmt.Errorf("cookie 池里 %d 个账号都不可用（最后一个：%w）；"+
				"到面板「Cookie 池」用「检测」按钮逐个排查，或打开 fallback_anon 降级匿名",
				len(tried), lastCookieErr))
		}
		config.Logf("[cookie] 试过的 %d 个账号都不可用，本次降级匿名（能力会退化到匿名档）", len(tried))
		cookieID, cookieLabel = 0, ""
	}
	// 图片附件：上传要 cookie，而且必须走跟正式请求同一个出口，所以排在这里。
	if len(pending) > 0 {
		if cookieStr == "" {
			return attrib(fmt.Errorf("image input needs a Google account cookie: " +
				"anonymous uploads succeed but referencing them in a conversation is " +
				"rejected upstream. Add a cookie in the admin panel (Cookie pool)"))
		}
		nImg, nVid := 0, 0
		for _, u := range pending {
			ref, uerr := gemini.UploadBytes(cookieStr, proxyURL, u.Data, u.Name)
			if uerr != nil {
				return attrib(fmt.Errorf("上传附件 %s 失败: %w", u.Name, uerr))
			}
			files = append(files, gemini.FileRef{Ref: ref, Name: u.Name, Kind: u.Kind, Mime: u.Mime})
			if u.Kind == 2 {
				nVid++
			} else {
				nImg++
			}
		}
		config.Logf("[vision] 上传了 %d 张图 / %d 个视频", nImg, nVid)
	}

	// prompt 超长时转成文本附件。要等挑完号和出口才能做：上传要 cookie，
	// 而且必须走跟正式请求同一个出口。
	budget := config.Runtime().MaxPromptBytes
	if p, f, used, ferr := prepareContextFile(prompt, latest, budget, cookieStr, proxyURL); ferr != nil {
		return attrib(ferr)
	} else if used {
		prompt = p
		files = append(files, f...)
	}
	if budget > 0 && len(prompt) > budget {
		return attrib(&PromptTooLongError{
			Bytes: len(prompt), Budget: budget, HasCookie: cookieStr != "",
		})
	}

	inner := gemini.NewModelPayload(mc)
	if len(files) > 0 {
		// 形状逐字取自浏览器抓包：
		//   [[[路径, 类型, null, mime], "文件名", null×6, [0]], …]
		// 类型位是 1=图片 / 3=文本文件 —— 拿 1 传文本文件等于告诉服务端"这是张图"。
		refs := make([]interface{}, 0, len(files))
		for _, f := range files {
			kind := f.Kind
			if kind == 0 {
				kind = 3
			}
			mime := f.Mime
			if mime == "" {
				mime = "text/plain"
			}
			refs = append(refs, []interface{}{
				[]interface{}{f.Ref, kind, nil, mime}, f.Name,
				nil, nil, nil, nil, nil, nil,
				[]interface{}{0},
			})
		}
		inner[0] = []interface{}{prompt, 0, nil, refs, nil, nil, 0}
	} else {
		inner[0] = []interface{}{prompt, 0, nil, nil, nil, nil, 0}
	}
	inner[1] = []interface{}{"en"}
	inner[2] = []interface{}{"", "", "", nil, nil, nil, nil, nil, nil, ""}
	inner[7] = 1
	inner[10] = 1
	inner[11] = 0
	// 会话内轮次索引；我们每次都是新会话，恒为 0。
	inner[17] = []interface{}{[]interface{}{0}}
	inner[18] = 0
	inner[27] = 1
	// 抓包里浏览器三种场景（有 cookie / 无 cookie / 扩展思考）全是 [1]。
	// 我们原来写 [2]，是早期抄来的值、协议层已被证伪。含义仍未知，
	// 匿名两个值都能通，但没有理由继续偏离浏览器。
	inner[41] = []interface{}{1}
	// 临时对话（#34）：inner[45]=1 让这条不进账号的会话列表、不入训练。只在有登录
	// cookie 时发——匿名本就不落账号，发了也没意义且没验过。multi_turn 开着时
	// TemporaryChatEligible 直接为假（两者冲突，见那里）。
	if TemporaryChatEligible() && cookieStr != "" {
		inner[45] = 1
	}
	inner[53] = 0
	reqUUID := uuid.NewString()
	inner[59] = reqUUID
	inner[61] = []interface{}{}
	// 媒体工具开关。填了服务端就换后端模型出图/出乐（响应里带产物引用，字节要另取）。
	if mc.Tool > 0 {
		inner[49] = mc.Tool
	}
	// 视频还要在 inner[55] 指定画幅比例：[[16]]=16:9，[[17]]=9:16（抓包）。
	if mc.Tool == gemini.ToolVideo {
		inner[55] = []interface{}{[]interface{}{16}}
	}

	innerJSON, err := json.Marshal(inner)
	if err != nil {
		return nil, err
	}
	outer := []interface{}{nil, string(innerJSON)}
	outerJSON, err := json.Marshal(outer)
	if err != nil {
		return nil, err
	}

	// 带 cookie 时必须多发一个表单字段 at（XSRF token），否则上游直接 400。
	// 匿名请求不需要，getXSRF 对空 cookie 返回空串。见 xsrf.go。
	buildBody := func(at string) string {
		form := url.Values{}
		form.Set("f.req", string(outerJSON))
		if at != "" {
			form.Set("at", at)
		}
		return form.Encode()
	}

	body := buildBody(xsrfToken)

	// 抓包里 header 下标 16 和 inner[59] 是两个不同的 uuid，各生成各的。
	modelHeader := gemini.BuildModelHeader(mc.HexID, mc.Mode, gemini.ModelThinkingValue(mc), uuid.NewString())
	sessionHeader := fmt.Sprintf(`["%s",1]`, reqUUID)

	geminiHeaders := gemini.BuildGeminiHeaders(cookieStr, sapisid, mc.HexID)
	geminiHeaders["x-goog-ext-525001261-jspb"] = modelHeader
	geminiHeaders["x-goog-ext-525005358-jspb"] = sessionHeader
	var lastErr error
	// 最后一次拿到的 HTTP 状态码，0 表示网络层就失败了没拿到响应。
	// cookie 健康度只认 401/403，别的状态不算 cookie 的错，见 markCookieByStatus。
	lastStatus := 0
	xsrfRetried := false // XSRF 自愈只做一次，避免死循环
	t0 := time.Now()

	tracker := &gemini.DeltaTracker{}
	rtracker := &gemini.DeltaTracker{}
	var lineCB func(string)
	if onDelta != nil || onReasoning != nil {
		lineCB = func(line string) {
			// 思考链先推完才轮到正文，所以先处理它，客户端拿到的顺序才对。
			if onReasoning != nil {
				if r := gemini.ReasoningInLine(line); r != "" {
					if d := rtracker.Push(r); d != "" {
						onReasoning(d)
					}
				}
			}
			if onDelta == nil {
				return
			}
			for _, t := range gemini.TextsInLine(line) {
				if d := tracker.Push(t); d != "" {
					onDelta(d)
				}
			}
		}
	}

	for attempt := 0; attempt < config.Runtime().RetryAttempts; attempt++ {
		statusCode, raw, ttfb, setCookie, err := gemini.DoGeminiRequest(endpoint, body, geminiHeaders, proxyURL, lineCB)
		if len(setCookie) > 0 && cookieID > 0 {
			if merged := cookies.MergeSetCookie(cookieStr, setCookie); merged != cookieStr {
				cookieStr = merged
				pool.UpdateAccountCookie(cookieID, merged)
			}
		}
		if err != nil {
			lastErr = err
			if pickedOK {
				pool.RecordProxyResult(picked.ID, false, err.Error())
			}
			// 已经往客户端吐过内容就不能重试，否则会重复（思考链也算吐过）。
			if tracker.Text() != "" || rtracker.Text() != "" {
				break
			}
			if attempt < config.Runtime().RetryAttempts-1 {
				config.Logf("retry %d/%d: %v", attempt+1, config.Runtime().RetryAttempts, err)
				time.Sleep(time.Duration(config.Runtime().RetryDelaySec) * time.Second)
			}
			continue
		}
		if statusCode != 200 {
			// token 过期时上游回 400 + xsrf。作废缓存重取一次再打，
			// 这种自愈不算进重试预算，否则一次过期就吃掉全部重试。
			if statusCode == 400 && gemini.IsXSRFError(string(raw)) && cookieStr != "" && !xsrfRetried {
				xsrfRetried = true
				gemini.InvalidateXSRF(cookieStr)
				if tok, e := gemini.GetXSRF(cookieStr, proxyURL); e == nil {
					body = buildBody(tok)
					geminiHeaders = gemini.BuildGeminiHeaders(cookieStr, sapisid, mc.HexID)
					geminiHeaders["x-goog-ext-525001261-jspb"] = modelHeader
					geminiHeaders["x-goog-ext-525005358-jspb"] = sessionHeader
					attempt--
					continue
				}
			}
			lastErr = fmt.Errorf("upstream HTTP %d: %s", statusCode, gemini.Truncate(string(raw), 200))
			lastStatus = statusCode
			if pickedOK {
				pool.RecordProxyResult(picked.ID, false, lastErr.Error())
			}
			if tracker.Text() != "" || rtracker.Text() != "" {
				break
			}
			if attempt < config.Runtime().RetryAttempts-1 {
				time.Sleep(time.Duration(config.Runtime().RetryDelaySec) * time.Second)
			}
			continue
		}
		// HTTP 200 但一个内容帧都没有 —— 上游的瞬时拒绝（响应里那个 1155）。
		// 它不是限流：干净 IP 间隔 1s 连打 15 次全过、同 IP 并发 10 共 18 次全过、
		// 打了 60+ 次的 IP 之后照样成功，没有可预测阈值，同样的请求有时成功有时失败。
		// 重发一次通常就好，所以必须纳入重试——不然一次抖动就变成客户端可见的 502。
		//
		// 判据是**有没有内容帧**，不是 BardErrorInfo：正常响应的结束帧里也带错误码
		// （1096 = 会话未持久化），拿它判错会把每个正常响应都判成失败。
		if !gemini.HasContentFrame(string(raw)) {
			lastErr = fmt.Errorf("upstream returned no content frame (raw %d bytes)", len(raw))
			if pickedOK {
				// 记进代理健康度：1155 跟出口质量强相关（干净出口 60+ 次 0 发生，
				// 脏出口一天约 9 次），连续踩中说明这个出口该歇了。
				pool.RecordProxyResult(picked.ID, false, lastErr.Error())
			}
			if tracker.Text() != "" || rtracker.Text() != "" {
				break
			}
			if attempt < config.Runtime().RetryAttempts-1 {
				config.Logf("retry %d/%d: 空响应（无内容帧，%d 字节）", attempt+1, config.Runtime().RetryAttempts, len(raw))
				time.Sleep(time.Duration(config.Runtime().RetryDelaySec) * time.Second)
			}
			continue
		}
		if pickedOK {
			pool.RecordProxyResult(picked.ID, true, "")
		}
		pool.MarkCookieByStatus(cookieID, 200, "")
		result := &gemini.StreamResult{
			Emitted:          tracker.Text(),
			EmittedReasoning: rtracker.Text(),
			Raw:              string(raw),
			Reasoning:        gemini.ExtractReasoning(string(raw)),
			UpstreamModel:    gemini.ExtractUpstreamModel(string(raw)),
			ProxyID:          picked.ID,
			ProxyName:        picked.Name,
			AccountID:        cookieID,
			AccountLabel:     cookieLabel,
			TTFBMs:           ttfb,
			TotalMs:          time.Since(t0).Milliseconds(),
		}
		// 媒体模型：生成的产物字节不在这条响应里，要用同一套 cookie / 出口再走一遍
		// hNvQHb + 下载 host 取回。取不到就记 MediaErr，让上层报错而不是返回半成品。
		if mc.Tool == gemini.ToolImage || mc.Tool == gemini.ToolMusic || mc.Tool == gemini.ToolVideo {
			mime := "image/png"
			switch mc.Tool {
			case gemini.ToolMusic:
				mime = "audio/mpeg"
			case gemini.ToolVideo:
				mime = "video/mp4"
			}
			arts, aerr := gemini.FetchMediaArtifacts(
				mc.Tool, string(raw), gemini.ExtractConversationID(string(raw)),
				cookieStr, sapisid, xsrfToken, proxyURL, mime)
			if aerr != nil {
				config.Logf("[media] 取回产物失败: %v", aerr)
				result.MediaErr = aerr.Error()
			} else {
				result.Artifacts = arts
				config.Logf("[media] 取回 %d 份产物", len(arts))
			}
		}
		// #19 自动删会话：出完结果把 gemini.google.com 上留下的这条会话删掉，避免
		// 用户账号里堆一堆。只登录态能删（要 XSRF），异步 best-effort，不影响响应。
		//
		// 临时对话（#34）生效时跳过：那条压根没进会话列表，删它是多发一次 RPC 删空气。
		// 这正是临时对话比 create-then-delete 省的那一次出站请求。
		if config.Runtime().AutoDeleteConversation && !TemporaryChatEligible() && cookieStr != "" && xsrfToken != "" {
			if cid := gemini.ExtractConversationID(string(raw)); cid != "" {
				go gemini.DeleteConversation(cid, cookieStr, sapisid, xsrfToken, proxyURL)
			}
		}
		return result, nil
	}
	if lastErr != nil {
		pool.MarkCookieByStatus(cookieID, lastStatus, lastErr.Error())
	}
	return attrib(lastErr)
}

// ProbeGemini 直接调 Gemini StreamGenerate（绕过限流），返回详细诊断。
// 不写 db、不消耗限流 slot。
func ProbeGemini(prompt, proxyURL string) gemini.ProbeResult {
	res := gemini.ProbeResult{Impersonate: config.Runtime().Impersonate}

	inner := make([]interface{}, gemini.InnerSlots)
	inner[0] = []interface{}{prompt, 0, nil, nil, nil, nil, 0}
	inner[1] = []interface{}{"en"}
	inner[2] = []interface{}{"", "", "", nil, nil, nil, nil, nil, nil, ""}
	inner[6] = []interface{}{0}
	inner[7] = 1
	inner[10] = 1
	inner[11] = 0
	inner[17] = []interface{}{[]interface{}{0}}
	inner[18] = 0
	inner[27] = 1
	inner[30] = []interface{}{4}
	// 抓包里浏览器三种场景（有 cookie / 无 cookie / 扩展思考）全是 [1]。
	// 我们原来写 [2]，是早期抄来的值、协议层已被证伪。含义仍未知，
	// 匿名两个值都能通，但没有理由继续偏离浏览器。
	inner[41] = []interface{}{1}
	inner[53] = 0
	inner[59] = uuid.NewString()
	inner[61] = []interface{}{}
	inner[68] = 1
	probeModel := gemini.Models["gemini-3.6-flash"]
	inner[79] = probeModel.Mode

	innerJSON, _ := json.Marshal(inner)
	outer := []interface{}{nil, string(innerJSON)}
	outerJSON, _ := json.Marshal(outer)
	form := url.Values{}
	form.Set("f.req", string(outerJSON))
	body := form.Encode()

	reqid := time.Now().Unix() % 1000000
	endpoint := fmt.Sprintf(
		"https://gemini.google.com/_/BardChatUi/data/assistant.lamda.BardFrontendService/StreamGenerate?bl=%s&hl=en&_reqid=%d&rt=c",
		gemini.CurrentBL(proxyURL), reqid,
	)

	// probe 是旁路探测，不回写 cookie 健康度：它的失败原因跟 cookie 无关。
	// 但 at 必须带——否则挂了 cookie 之后连通性探测会一直报 400，假报故障。
	cookieStr, sapisid := loadCookie()
	if tok, e := gemini.GetXSRF(cookieStr, proxyURL); e == nil && tok != "" {
		form.Set("at", tok)
		body = form.Encode()
	}
	headers := gemini.BuildGeminiHeaders(cookieStr, sapisid, probeModel.HexID)

	// 复用主流程同款 client 选择规则:有代理走 stdlib，没代理走 tls-client。
	// 但 probe 需要看 302 的 Location header,所以这里直接发不用 doGeminiRequest。
	var statusCode int
	var raw []byte
	var locHeader string
	var err error

	if proxyURL != "" {
		req, e := http.NewRequest("POST", endpoint, strings.NewReader(body))
		if e != nil {
			res.Status = "network_error"
			res.Diagnostic = "构建请求失败: " + e.Error()
			return res
		}
		transport.ApplyBrowserHeaders(req.Header)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		client := transport.ProxyClient(proxyURL)
		resp, e := client.Do(req)
		if e != nil {
			res.Status = "network_error"
			res.Diagnostic = "网络层错误（DNS/TCP/TLS 失败）: " + e.Error()
			return res
		}
		defer resp.Body.Close()
		statusCode = resp.StatusCode
		locHeader = resp.Header.Get("Location")
		raw, err = io.ReadAll(resp.Body)
	} else {
		req, e := fhttp.NewRequest("POST", endpoint, strings.NewReader(body))
		if e != nil {
			res.Status = "network_error"
			res.Diagnostic = "构建请求失败: " + e.Error()
			return res
		}
		transport.ApplyBrowserHeaders(req.Header)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		client := transport.TLSClient()
		resp, e := client.Do(req)
		if e != nil {
			res.Status = "network_error"
			res.Diagnostic = "网络层错误（DNS/TCP/TLS 失败）: " + e.Error()
			return res
		}
		defer resp.Body.Close()
		statusCode = resp.StatusCode
		locHeader = resp.Header.Get("Location")
		raw, err = io.ReadAll(resp.Body)
	}
	_ = err
	res.HTTPCode = statusCode
	res.UpstreamSnip = gemini.Truncate(string(raw), 300)

	switch {
	case statusCode == 302:
		res.Status = "blocked_sorry"
		res.Diagnostic = "IP 被 Google 风控（重定向到 sorry/index）。" +
			"通常 6-24 小时解除，或换 VPN/代理 IP 立即恢复。Location: " + gemini.Truncate(locHeader, 200)
		return res
	case statusCode == 429:
		res.Status = "rate_limited"
		res.Diagnostic = "Google 直接返回 429 限流。同样是 IP 嫌疑，但风控分支不同（朴素 SDK 路径）。"
		return res
	case statusCode != 200:
		res.Status = "upstream_error"
		res.Diagnostic = fmt.Sprintf("上游返回非 200 (HTTP %d)，可能是协议变更或临时故障。", statusCode)
		return res
	}

	text := gemini.ExtractResponseText(string(raw))
	if text == "" {
		res.Status = "upstream_error"
		res.Diagnostic = "上游 200 但只回了结束帧、没有内容帧，通常是请求被服务端拒绝" +
			"（例如带了不被接受的会话 id 或工具开关），不是帧格式变更。"
		return res
	}

	res.OK = true
	res.Status = "success"
	res.ResponseText = gemini.Truncate(text, 200)
	res.Diagnostic = "调用成功。延迟 / 内容见上面字段。"
	return res
}
