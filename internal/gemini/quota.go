package gemini

// 网页用量 RPC jSf9Qc 的结构与解析判据参考 MIT 项目 Nagi-ovo/voyager：
// https://github.com/Nagi-ovo/voyager
// public/usage-observer.js 与 src/pages/content/usageStatus/usageParsing.ts。
// 逆向协议可能变化；本能力独立于生成主链路。

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"

	"gemini-web2api/internal/cookies"
)

// QuotaMetric 是一档账号额度；允许上游报告最高 150% 的超额用量。
type QuotaMetric struct {
	UsedPercent int   `json:"used_percent"`
	RemainPct   int   `json:"remain_percent"`
	ResetAt     int64 `json:"reset_at"`
	ResetInSec  int64 `json:"reset_in_sec"`
}

// FetchQuota 使用调用方指定的凭据和出口查询，不选择账号或修改资源池。
func FetchQuota(cookie, proxyURL string) (five, weekly *QuotaMetric, err error) {
	xsrf, err := GetXSRF(cookie, proxyURL)
	if err != nil || xsrf == "" {
		// 不向管理响应回传可能含代理凭据的底层网络错误。
		return nil, nil, fmt.Errorf("取 XSRF token 失败，请检查账号登录态与出口连接")
	}
	query := url.Values{
		"rpcids": {"jSf9Qc"}, "bl": {CurrentBL(proxyURL)}, "hl": {"en"},
		"_reqid": {fmt.Sprint(time.Now().UnixNano() % 1000000)},
		"rt":     {"c"}, "source-path": {"/usage"},
	}
	form := url.Values{"f.req": {`[[["jSf9Qc","[]",null,"generic"]]]`}, "at": {xsrf}}
	headers := BuildGeminiHeaders(cookie, cookies.SAPISID(cookie), "")
	delete(headers, "x-goog-ext-525001261-jspb")
	status, _, body, err := uploadPost(
		"https://gemini.google.com/_/BardChatUi/data/batchexecute?"+query.Encode(),
		headers, []byte(form.Encode()), proxyURL)
	if err != nil {
		return nil, nil, fmt.Errorf("额度查询网络失败，请检查出口连接后重试")
	}
	if status != 200 {
		return nil, nil, fmt.Errorf("额度查询上游 HTTP %d，请检查登录态与出口连接", status)
	}
	return ParseQuotaBody(string(body))
}

// ParseQuotaBody 遍历匹配 RPC 的数组载荷，不依赖固定桶位置，忽略未知档位和无关帧。
func ParseQuotaBody(raw string) (five, weekly *QuotaMetric, err error) {
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "[[") {
			continue
		}
		var frame [][]json.RawMessage
		if json.Unmarshal([]byte(line), &frame) != nil {
			continue
		}
		for _, part := range frame {
			if len(part) < 3 {
				continue
			}
			var tag, rpc, payload string
			if json.Unmarshal(part[0], &tag) != nil || tag != "wrb.fr" ||
				json.Unmarshal(part[1], &rpc) != nil || rpc != "jSf9Qc" ||
				json.Unmarshal(part[2], &payload) != nil {
				continue
			}
			var root interface{}
			if json.Unmarshal([]byte(payload), &root) != nil {
				continue
			}
			pending := []interface{}{root}
			for len(pending) > 0 {
				v := pending[len(pending)-1]
				pending = pending[:len(pending)-1]
				items, ok := v.([]interface{})
				if !ok {
					continue
				}
				if metric, period := readQuotaMetric(items); metric != nil {
					if period == 1 && five == nil {
						five = metric
					} else if period == 2 && weekly == nil {
						weekly = metric
					}
					continue
				}
				for i := len(items) - 1; i >= 0; i-- {
					pending = append(pending, items[i])
				}
			}
		}
	}
	if five == nil && weekly == nil {
		return nil, nil, fmt.Errorf("额度字段解析失败：上游未提供可识别的档位，可能尚未开放额度页面或协议已变更")
	}
	return five, weekly, nil
}

func readQuotaMetric(items []interface{}) (*QuotaMetric, int) {
	if len(items) < 4 {
		return nil, 0
	}
	limit, ok := items[0].(float64)
	if !ok || limit <= 0 {
		return nil, 0
	}
	frac, ok := items[1].(float64)
	if !ok || frac < 0 || frac > 1.5 {
		return nil, 0
	}
	period, ok := items[2].(float64)
	if !ok || (period != 1 && period != 2) {
		return nil, 0
	}
	reset, ok := items[3].([]interface{})
	if !ok || len(reset) == 0 {
		return nil, 0
	}
	head, ok := reset[0].([]interface{})
	if !ok || len(head) == 0 {
		return nil, 0
	}
	epoch, ok := head[0].(float64)
	if !ok || epoch < 1_600_000_000 || epoch > 253_402_300_799 || math.Trunc(epoch) != epoch {
		return nil, 0
	}
	used := int(math.Round(frac * 100))
	return &QuotaMetric{UsedPercent: used, RemainPct: max(0, 100-used), ResetAt: int64(epoch)}, int(period)
}
