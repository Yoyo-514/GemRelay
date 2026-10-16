package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gemrelay/internal/config"
)

func TestLoadExplicitConfig(t *testing.T) {
	old := *config.Startup()
	t.Cleanup(func() { *config.Startup() = old })
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"port":9083,"host":"127.0.0.1","multi_turn":true,"strict_model":true,"quota_aware_pick":true,"quota_poll_interval_sec":90}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := config.Load(path); err != nil {
		t.Fatal(err)
	}
	got := config.Startup()
	if got.Port != 9083 || got.Host != "127.0.0.1" || !got.MultiTurn || !got.StrictModel {
		t.Fatalf("配置未加载: %+v", got)
	}
	if !got.QuotaAwarePick || got.QuotaPollIntervalSec != 90 {
		t.Fatalf("额度配置应按文件加载: %+v", got)
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
	if value.QuotaAwarePick || value.QuotaPollIntervalSec != 60 {
		t.Fatal("模板应采用默认选号策略和 60 秒轮询间隔")
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

func TestGemRelayConfigSearch(t *testing.T) {
	for _, c := range []struct {
		name    string
		files   map[string]string
		port    int
		wantErr bool
	}{
		{"defaults", nil, 8083, false},
		{"home directory ignored", map[string]string{"home": `{"port":9081}`}, 8083, false},
		{"working directory first", map[string]string{"home": `{"port":9081}`, "local": `{"port":9082}`}, 9082, false},
		{"old directory ignored", map[string]string{"old": `{"port":9083}`}, 8083, false},
		{"invalid home config ignored", map[string]string{"old": `{"port":9083}`, "home": `{`}, 8083, false},
		{"invalid local config reported", map[string]string{"local": `{`}, 0, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			old := *config.Startup()
			t.Cleanup(func() { *config.Startup() = old })
			home, work := t.TempDir(), t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Chdir(work)
			paths := map[string]string{
				"local": filepath.Join(work, "config.json"),
				"home":  filepath.Join(home, ".config", "gemrelay", "config.json"),
				"old":   filepath.Join(home, ".config", "gemini-web2api", "config.json"),
			}
			for location, body := range c.files {
				path := paths[location]
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
			}
			err := config.Load("")
			if c.wantErr {
				if err == nil {
					t.Fatal("无效配置应返回错误")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := config.Startup(); got.Port != c.port || got.DBPath != "./data/gemrelay.db" {
				t.Fatalf("配置查找或数据库默认值错误: %+v", got)
			}
		})
	}
}
