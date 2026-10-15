package tests

import (
	"strings"
	"testing"
	"time"

	"gemrelay/internal/gateway"
)

func TestVideoJobFailureAndSnapshot(t *testing.T) {
	initial := gateway.StartVideo("hi", "missing-model")
	if initial.Status != "queued" || initial.Object != "video" {
		t.Fatalf("初始状态错误: %+v", initial)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		current := gateway.VideoJobByID(initial.ID)
		if current == nil {
			t.Fatal("任务未保存")
		}
		if current.Status == "failed" {
			if !strings.Contains(current.Error, "unknown model") {
				t.Errorf("缺少原始失败原因: %q", current.Error)
			}
			if initial.Status != "queued" {
				t.Error("初始快照被后台修改")
			}
			current.Status = "changed-by-caller"
			if gateway.VideoJobByID(initial.ID).Status != "failed" {
				t.Error("调用方修改泄漏到任务状态")
			}
			data, mime := gateway.VideoJobByID(initial.ID).Content()
			if len(data) != 0 || mime != "" {
				t.Error("失败任务不应有视频产物")
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("本地模型校验任务未及时结束")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestUnknownVideoJob(t *testing.T) {
	if gateway.VideoJobByID("missing") == nil {
		return
	}
	t.Fatal("不存在的任务不应返回状态")
}
