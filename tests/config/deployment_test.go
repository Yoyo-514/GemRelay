package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestContainerBuildContract(t *testing.T) {
	read := func(path string) string {
		t.Helper()
		body, err := os.ReadFile(filepath.Join("../..", path))
		if err != nil {
			t.Fatal(err)
		}
		return strings.ReplaceAll(string(body), "\r\n", "\n")
	}
	dockerfile := read("build/package/Dockerfile")
	module := read("go.mod")
	goVersion := regexp.MustCompile(`(?m)^go ([^\s]+)$`).FindStringSubmatch(module)
	buildVersion := regexp.MustCompile(`(?m)^ARG GO_VERSION=([^\s]+)$`).FindStringSubmatch(dockerfile)
	if len(goVersion) != 2 || len(buildVersion) != 2 || goVersion[1] != buildVersion[1] {
		t.Fatal("模块与镜像构建的默认 Go 版本必须一致")
	}
	for _, source := range []string{"cmd", "internal", "web"} {
		if !strings.Contains(dockerfile, "COPY "+source+"/ ./"+source+"/") {
			t.Errorf("镜像缺少构建输入 %s", source)
		}
	}
	if strings.Contains(dockerfile, "COPY . .") || strings.Contains(dockerfile, "ENV DB_PATH=") {
		t.Fatal("不应复制整个本机工作区或声明未读取的数据库环境变量")
	}
	match := regexp.MustCompile(`(?m)^CMD (\[.*\])$`).FindStringSubmatch(dockerfile)
	if len(match) != 2 {
		t.Fatal("容器启动参数缺失")
	}
	var command []string
	if err := json.Unmarshal([]byte(match[1]), &command); err != nil {
		t.Fatal(err)
	}
	if strings.Join(command, " ") != "--db /data/gemrelay.db --port 8083" {
		t.Fatalf("持久化路径或监听端口发生意外变化: %v", command)
	}
	ignore := read(".dockerignore")
	for _, line := range []string{"**", "!go.mod", "!go.sum", "!cmd/**", "!internal/**", "!web/**"} {
		if !strings.Contains("\n"+ignore, "\n"+line+"\n") {
			t.Errorf("构建上下文允许列表缺少 %s", line)
		}
	}
}

func TestComposeBuildsCurrentCheckout(t *testing.T) {
	body, err := os.ReadFile("../../deployments/docker-compose.yml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, want := range []string{
		"image: gemrelay:local", "pull_policy: build", "context: ..",
		"dockerfile: build/package/Dockerfile", "${ADMIN_TOKEN:?", "127.0.0.1:8083:8083", "gemrelay-data:/data",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("部署契约缺少 %q", want)
		}
	}
	if strings.Contains(text, "ghcr.io/zexadev/") || strings.Contains(text, "change-me-in-production") {
		t.Fatal("本仓 Compose 不应默认运行上游镜像或公开默认令牌")
	}
	context := filepath.Clean(filepath.Join("../../deployments", ".."))
	if _, err := os.Stat(filepath.Join(context, "build/package/Dockerfile")); err != nil {
		t.Fatalf("Compose 构建上下文中的镜像定义不可读: %v", err)
	}
}
