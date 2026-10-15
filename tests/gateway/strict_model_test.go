package tests

import (
	"errors"
	"strings"
	"testing"

	"gemini-web2api/internal/config"
	"gemini-web2api/internal/gateway"
	"gemini-web2api/internal/gemini"
	"gemini-web2api/internal/storage"
	"gemini-web2api/tests/testutil"
)

func TestStrictModelViolation(t *testing.T) {
	old := config.Runtime()
	t.Cleanup(func() { config.ApplyRuntime(old) })
	result := &gemini.StreamResult{UpstreamModel: "3.5 Flash-Lite"}
	rt := old
	rt.StrictModel = false
	config.ApplyRuntime(rt)
	if err := gateway.StrictModelViolation("gemini-4-argon", result); err != nil {
		t.Fatal("默认关闭不能改变响应", err)
	}
	rt.StrictModel = true
	config.ApplyRuntime(rt)
	err := gateway.StrictModelViolation("gemini-4-argon-high", result)
	var mismatch *gateway.ModelDowngradeError
	if !errors.As(err, &mismatch) {
		t.Fatalf("没有可分类的模型不匹配错误: %v", err)
	}
	for _, want := range []string{"gemini-4-argon-high", "3.5 Flash-Lite", "4 argon"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("错误缺少证据 %q: %v", want, err)
		}
	}
	for _, result := range []*gemini.StreamResult{nil, {}, {UpstreamModel: "4 Argon High"}} {
		if err := gateway.StrictModelViolation("gemini-4-argon-high", result); err != nil {
			t.Errorf("缺失证据或匹配时不应失败: %v", err)
		}
	}
}

func TestStrictModelRuntimePersistence(t *testing.T) {
	testutil.ResetState(t)
	old, startup := config.Runtime(), *config.Startup()
	t.Cleanup(func() { *config.Startup() = startup; config.ApplyRuntime(old) })
	config.Startup().StrictModel = true
	gateway.InitRuntimeConfig()
	if !config.Runtime().StrictModel {
		t.Fatal("启动配置没有进入运行时")
	}
	if err := storage.SetValue(config.RuntimeConfigKey, `{"strict_model":false}`); err != nil {
		t.Fatal(err)
	}
	// 测试进程启动配置不保证完整，使用实际模板补齐合法基线。
	if err := config.Load("../../configs/config.example.json"); err != nil {
		t.Fatal(err)
	}
	config.Startup().DBPath = startup.DBPath
	config.Startup().StrictModel = true
	gateway.InitRuntimeConfig()
	if config.Runtime().StrictModel {
		t.Fatal("持久化关闭值应覆盖启动开启值")
	}
	next := config.Runtime()
	next.StrictModel = true
	if err := gateway.SaveRuntimeConfig(next); err != nil {
		t.Fatal(err)
	}
	if !config.Runtime().StrictModel || !strings.Contains(storage.GetValue(config.RuntimeConfigKey), `"strict_model":true`) {
		t.Fatal("保存后必须持久化并发布严格模型开关")
	}
}
