package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gemini-web2api/internal/config"
)

func TestLoadExplicitConfig(t *testing.T) {
	old := *config.Startup()
	t.Cleanup(func() { *config.Startup() = old })
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"port":9083,"host":"127.0.0.1","multi_turn":true,"strict_model":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := config.Load(path); err != nil {
		t.Fatal(err)
	}
	got := config.Startup()
	if got.Port != 9083 || got.Host != "127.0.0.1" || !got.MultiTurn || !got.StrictModel {
		t.Fatalf("配置未加载: %+v", got)
	}
	if got.DefaultModel != "gemini-3.6-flash" {
		t.Error("未指定字段必须保留默认值")
	}
}

func TestLoadConfigErrors(t *testing.T) {
	old := *config.Startup()
	t.Cleanup(func() { *config.Startup() = old })
	for _, body := range []string{"{", `{"port":"invalid"}`} {
		path := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		if err := config.Load(path); err == nil {
			t.Errorf("无效配置应返回错误: %s", body)
		}
	}
	if err := config.Load(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Error("显式路径不存在时应返回错误")
	}
}

func TestConfigTemplate(t *testing.T) {
	body, err := os.ReadFile("../../configs/config.example.json")
	if err != nil {
		t.Fatal(err)
	}
	var value config.Config
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatal(err)
	}
	if value.Port <= 0 || value.DefaultModel == "" {
		t.Fatal("配置模板缺少启动字段")
	}
	if value.StrictModel {
		t.Fatal("配置模板中的严格模型校验必须默认关闭")
	}
}

func TestRuntimeSnapshot(t *testing.T) {
	old := config.Runtime()
	t.Cleanup(func() { config.ApplyRuntime(old) })
	next := old
	next.DefaultModel = "snapshot-model"
	config.ApplyRuntime(next)
	snapshot := config.Runtime()
	snapshot.DefaultModel = "local-change"
	if got := config.Runtime().DefaultModel; got != "snapshot-model" {
		t.Fatalf("快照修改泄漏: %q", got)
	}
}
