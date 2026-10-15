package tests

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"gemrelay/internal/cookies"
	"gemrelay/internal/gemini"
)

// 哨兵必须是这串 JSPB，不能走 json.Marshal：后者会把 000 收成 0，服务端不认。
func TestRotate1PSIDTSBodyIsJSPBSentinel(t *testing.T) {
	const want = `[000,"-0000000000000000000"]`
	_, body := gemini.BuildCookieRotation("")
	if body != want {
		t.Fatalf("payload = %q，期望 %q", body, want)
	}
	b, err := json.Marshal([]interface{}{0, "-0000000000000000000"})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) == body {
		t.Fatal("json.Marshal 的结果碰巧跟哨兵一样了，这个测试该改")
	}
}

func TestCookieSubsetKeepsOnlyNamed(t *testing.T) {
	in := "SID=a; __Secure-1PSID=psid; SAPISID=sap; __Secure-1PSIDTS=ts; NID=n"
	got := cookies.Subset(in, []string{"__Secure-1PSID", "__Secure-1PSIDTS"})
	if got != "__Secure-1PSID=psid; __Secure-1PSIDTS=ts" {
		t.Fatalf("got %q", got)
	}
}

func TestCookieSubsetSkipsEmptyAndDup(t *testing.T) {
	in := "__Secure-1PSID=psid; __Secure-1PSIDTS=; __Secure-1PSID=other"
	got, _ := gemini.BuildCookieRotation(in)
	if got != "__Secure-1PSID=psid" {
		t.Fatalf("got %q", got)
	}
}

func TestCookieValue(t *testing.T) {
	in := "SID=a; __Secure-1PSID=psid==; SAPISID=sap"
	if got := cookies.Value(in, "__Secure-1PSID"); got != "psid==" {
		t.Fatalf("got %q", got)
	}
	if got := cookies.Value(in, "nope"); got != "" {
		t.Fatalf("missing should be empty, got %q", got)
	}
}

func TestMergeSetCookieRefreshes1PSIDTS(t *testing.T) {
	base := "SID=a; __Secure-1PSID=psid; __Secure-1PSIDTS=oldts; SAPISID=sap"
	got := cookies.MergeSetCookie(base, []string{
		"__Secure-1PSIDTS=newts; Domain=.google.com; Secure; HttpOnly",
		"__Secure-3PSIDTS=new3; Domain=.google.com; Secure; HttpOnly",
	})
	if !strings.Contains(got, "__Secure-1PSIDTS=newts") {
		t.Errorf("1PSIDTS 没刷新: %s", got)
	}
	if strings.Contains(got, "oldts") {
		t.Errorf("旧 1PSIDTS 还在: %s", got)
	}
	if !strings.Contains(got, "__Secure-3PSIDTS=new3") {
		t.Errorf("3PSIDTS 没追加: %s", got)
	}
	for _, want := range []string{"SID=a", "__Secure-1PSID=psid", "SAPISID=sap"} {
		if !strings.Contains(got, want) {
			t.Errorf("丢了 %q: %s", want, got)
		}
	}
}

func TestAllow1PSIDTSRotateThrottle(t *testing.T) {
	const id int64 = -42
	var limiter gemini.RotationLimiter
	now := time.Now()
	if ok, _ := limiter.Allow(id, now); !ok {
		t.Fatal("第一次应该放行")
	}
	limiter.Note(id, nil, now)
	ok, wait := limiter.Allow(id, now)
	if ok {
		t.Fatal("60s 内应拦截")
	}
	if wait <= 0 || wait > 60*time.Second {
		t.Fatalf("wait=%s", wait)
	}
}

func TestNote1PSIDTSAttemptSkipsNetworkError(t *testing.T) {
	const id int64 = -43
	var limiter gemini.RotationLimiter
	now := time.Now()
	limiter.Note(id, fmt.Errorf("dial tcp timeout"), now)
	if ok, _ := limiter.Allow(id, now); !ok {
		t.Fatal("网络错误不该记节流")
	}
	limiter.Note(id, fmt.Errorf("RotateCookies 1PSIDTS 返回 HTTP 401（x）"), now)
	if ok, _ := limiter.Allow(id, now); ok {
		t.Fatal("401 应该记节流")
	}
}

func TestUniqueKeepOrder(t *testing.T) {
	got := gemini.UniqueKeepOrder([]string{"SIDCC", "__Secure-1PSIDTS", "SIDCC", "", "__Secure-1PSIDTS"})
	if strings.Join(got, ",") != "SIDCC,__Secure-1PSIDTS" {
		t.Fatalf("got %v", got)
	}
}

func TestMin1PSIDTSInterval(t *testing.T) {
	var limiter gemini.RotationLimiter
	now := time.Now()
	limiter.Note(1, nil, now)
	if ok, wait := limiter.Allow(1, now.Add(59*time.Second)); ok || wait != time.Second {
		t.Fatalf("60 秒内必须节流: ok=%v wait=%s", ok, wait)
	}
	if ok, wait := limiter.Allow(1, now.Add(60*time.Second)); !ok || wait != 0 {
		t.Fatalf("达到 60 秒应放行: ok=%v wait=%s", ok, wait)
	}
}
