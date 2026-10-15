package gemini

import (
	"regexp"
	"strings"
)

// 按协议标识归组，别名与思考档位共用判据。媒体工具可能切换后端，不参与校验。
// 显示名来自响应帧，不是客户端响应中回填的请求模型名。
var strictModelExpect = map[string][]string{
	hexArgon4:    {"4 argon"},
	hexFlash38:   {"3.8 flash", "3.7 flash"},
	hexFlash36:   {"3.6 flash"},
	hexPro31:     {"3.1 pro"},
	hexFlashLite: {"3.5 flash lite"},
}

var modelNameSeparators = regexp.MustCompile(`[^a-z0-9.]+`)

func normalizeModelName(value string) string {
	return strings.TrimSpace(modelNameSeparators.ReplaceAllString(strings.ToLower(value), " "))
}

// ModelDowngraded 只报告已知模型与上游显示名的不匹配；缺失证据或未知模型时放行。
// 不根据显示名推断思考级别，也不把不匹配等同于已证实的额度耗尽。
func ModelDowngraded(model, upstream string) (bool, string) {
	mc, ok := Models[model]
	if !ok || mc.Tool != 0 || strings.TrimSpace(upstream) == "" {
		return false, ""
	}
	want := strictModelExpect[mc.HexID]
	hay := " " + normalizeModelName(upstream) + " "
	for _, name := range want {
		if strings.Contains(hay, " "+normalizeModelName(name)+" ") {
			return false, ""
		}
	}
	if len(want) == 0 {
		return false, ""
	}
	return true, want[0]
}
