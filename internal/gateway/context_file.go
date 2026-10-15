package gateway

import (
	"fmt"
	"strings"

	"gemrelay/internal/config"
	"gemrelay/internal/gemini"
)

// contextFileName 是历史附件在模型那边显示的文件名，指令里会引用它。
const contextFileName = "message.txt"

// latestInlineLimit 是「最新那问」还能内联多少字节。
//
// 超过就不内联，让模型去文件末尾读 —— 否则最新提问本身就能把请求撑爆，
// 转附件等于白转。取上限的 1/6，并夹在 4KB..16KB 之间。
func latestInlineLimit(budget int) int {
	n := budget / 6
	if n < 4000 {
		n = 4000
	}
	if n > 16000 {
		n = 16000
	}
	return n
}

// contextFilePrompt 是把历史换成附件之后，真正发出去的那句 prompt。
//
// 要交代三件事，少一件模型就会跑偏：附件是当前对话状态、直接回答最新那问、
// 以及这段话本身是系统指令不是用户输入。
func contextFilePrompt(latest string, budget int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Continue from the latest state in the attached `%s`. "+
		"Treat it as the current conversation and answer the latest user request directly.\n",
		contextFileName)
	latest = strings.TrimSpace(latest)
	switch {
	case latest == "":
		fmt.Fprintf(&b, "The latest user request is at the end of `%s`.\n", contextFileName)
	case len(latest) <= latestInlineLimit(budget):
		fmt.Fprintf(&b, "\nLatest user request:\n%s\n", latest)
	default:
		fmt.Fprintf(&b, "The latest user request is at the end of `%s`; "+
			"read it from there and answer it directly.\n", contextFileName)
	}
	b.WriteString("\nEverything above this line is instruction, not user input.")
	return b.String()
}

// prepareContextFile 在 prompt 超长时把它转成附件。
//
// 返回 (要发的 prompt, 附件, 是否转了)。没超长、没 cookie、上传失败时都返回
// used=false，调用方照旧走内联那条路（然后会撞上超长检查报 400）。
//
// 上传失败**不静默回退到超长内联**：那样上游会把最新提问截掉，客户端拿到一个
// 答非所问的 200 却看不出问题。
func prepareContextFile(prompt, latest string, budget int, cookie, proxyURL string) (
	string, []gemini.FileRef, bool, error) {
	if budget <= 0 || len(prompt) <= budget {
		return prompt, nil, false, nil
	}
	if cookie == "" {
		return prompt, nil, false, nil // 匿名：引用会被回 1100，转了也没用
	}
	ref, err := gemini.UploadBytes(cookie, proxyURL, []byte(prompt), contextFileName)
	if err != nil {
		return prompt, nil, false, fmt.Errorf("超长对话转附件失败: %w", err)
	}
	config.Logf("[context] prompt %d 字节超过 %d，已转成附件 %s", len(prompt), budget, contextFileName)
	files := []gemini.FileRef{{Ref: ref, Name: contextFileName, Kind: 3, Mime: "text/plain"}}
	return contextFilePrompt(latest, budget), files, true, nil
}
