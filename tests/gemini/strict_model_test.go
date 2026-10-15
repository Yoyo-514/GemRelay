package tests

import (
	"testing"

	"gemini-web2api/internal/gemini"
)

func TestModelDowngraded(t *testing.T) {
	for _, c := range []struct {
		model, upstream string
		want            bool
	}{
		{"gemini-4-argon", "4 Argon High", false},
		{"gemini-4-argon-low", "4 Argon", false},
		{"gemini-4-argon-medium", "4 Argon High", false},
		{"gemini-4-argon-high", "3.5 Flash-Lite", true},
		{"gemini-3.5-flash-lite", "Gemini 3.5 FLASH-LITE", false},
		{"gemini-3.8-flash", "3.7 Flash", false},
		{"gemini-3.7-flash-thinking", "3.8 Flash", false},
		{"gemini-3.8-flash-thinking", "3.5 Flash-Lite", true},
		{"gemini-3.1-pro-thinking", "3.1 Pro", false},
		{"gemini-3.1-pro", "13.1 Pro", true},
		{"gemini-3.1-pro", "3.1 Professional", true},
		{"gemini-3.1-pro", "3.5 Flash-Lite", true},
		{"gemini-3.6-flash", "3.6 Flash", false},
		{"gemini-4-argon", "", false},
		{"gemini-4-argon", "  \t ", false},
		{"unknown", "3.5 Flash-Lite", false},
		{"gemini-image", "Nano Banana", false},
		{"gemini-video", "3.5 Flash-Lite", false},
	} {
		t.Run(c.model+"/"+c.upstream, func(t *testing.T) {
			got, expected := gemini.ModelDowngraded(c.model, c.upstream)
			if got != c.want {
				t.Fatalf("got=%v want=%v expected=%q", got, c.want, expected)
			}
			if got && expected == "" {
				t.Fatal("不匹配必须返回期望模型证据")
			}
		})
	}
}

func TestStrictModelCoversTextModels(t *testing.T) {
	for name, mc := range gemini.Models {
		if mc.Tool != 0 {
			continue
		}
		if bad, want := gemini.ModelDowngraded(name, "unrelated model"); !bad || want == "" {
			t.Errorf("文本模型缺少判据: %s", name)
		}
	}
}
