package tests

import (
	"encoding/json"
	"strings"
	"testing"

	"gemini-web2api/internal/gemini"
)

func quotaFrame(rpc, payload string) string {
	encoded, _ := json.Marshal([]interface{}{[]interface{}{"wrb.fr", rpc, payload, nil, nil, nil, "generic"}})
	return string(encoded)
}

func TestParseQuotaBody(t *testing.T) {
	for _, c := range []struct {
		name, payload string
		five, weekly  int
	}{
		{"sample", `[[[100,0.63,1,[[1792050755,0]]],[100,0.13,2,[[1792486355,0]]]]]`, 63, 13},
		{"single", `[[[100,0.4,1,[[1792050755,0]]]]]`, 40, -1},
		{"weekly", `[[[100,1,2,[[1792486355,0]]]]]`, -1, 100},
		{"deep nesting", `[null,[[null,[[100,0,1,[[1792050755,0]]]]]]]`, 0, -1},
		{"unknown bucket", `[[[100,0.9,3,[[1792050755,0]]],[100,0.13,2,[[1792486355,0]]]]]`, -1, 13},
		{"over quota", `[[[100,1.5,1,[[1792050755,0]]]]]`, 150, -1},
		{"rounding", `[[[100,0.635,1,[[1792050755,0]]]]]`, 64, -1},
	} {
		t.Run(c.name, func(t *testing.T) {
			raw := ")]}'\n\n1000\n" + quotaFrame("wrong-rpc", c.payload) + "\n" +
				quotaFrame("jSf9Qc", "[[1,2,3]]") + "\n" + quotaFrame("jSf9Qc", c.payload) + "\n12\n[[\"di\",42]]"
			five, weekly, err := gemini.ParseQuotaBody(raw)
			if err != nil {
				t.Fatal(err)
			}
			for i, metric := range []*gemini.QuotaMetric{five, weekly} {
				want := []int{c.five, c.weekly}[i]
				if want < 0 {
					if metric != nil {
						t.Errorf("未提供档位不应伪造: %+v", metric)
					}
					continue
				}
				if metric == nil || metric.UsedPercent != want || metric.RemainPct != max(0, 100-want) {
					t.Fatalf("metric=%+v want used=%d", metric, want)
				}
				if metric.ResetAt < 1_600_000_000 {
					t.Fatalf("无效重置时间: %+v", metric)
				}
			}
		})
	}
}

func TestQuotaRejectsInvalidPayloads(t *testing.T) {
	for _, payload := range []string{
		`[[[100,-0.1,1,[[1792050755,0]]]]]`,
		`[[[100,1.6,1,[[1792050755,0]]]]]`,
		`[[[100,0.5,1.9,[[1792050755,0]]]]]`,
		`[[[100,0.5,3,[[1792050755,0]]]]]`,
		`[[[100,0.5,1,[[1,0]]]]]`,
		`[[[100,0.5,1,[[1e30,0]]]]]`,
		`[[[100,0.5,1,[[1792050755.5,0]]]]]`,
		`[[[100,0.5,1,[]]]]`,
		`[[[null,0.5,1,[[1792050755,0]]]]]`,
		`[[[100,"0.5",1,[[1792050755,0]]]]]`,
		`[[1,2,3]]`,
	} {
		if _, _, err := gemini.ParseQuotaBody(quotaFrame("jSf9Qc", payload)); err == nil {
			t.Errorf("无效载荷被接受: %s", payload)
		}
	}
	for _, raw := range []string{"", "not json", ")]}'\n\n7\n", `[["wrb.fr"]]`,
		quotaFrame("other", `[[[100,0.63,1,[[1792050755,0]]]]]`), strings.Repeat("[", 20)} {
		if _, _, err := gemini.ParseQuotaBody(raw); err == nil {
			t.Errorf("无关或损坏响应被接受: %q", raw)
		}
	}
}

func TestQuotaBucketsAcrossFrames(t *testing.T) {
	raw := quotaFrame("jSf9Qc", `[[[100,0.63,1,[[1792050755,0]]]]]`) + "\n" +
		quotaFrame("jSf9Qc", `[[[100,0.13,2,[[1792486355,0]]]]]`)
	five, weekly, err := gemini.ParseQuotaBody(raw)
	if err != nil || five == nil || weekly == nil {
		t.Fatalf("跨帧档位未汇总: %+v %+v %v", five, weekly, err)
	}
}
