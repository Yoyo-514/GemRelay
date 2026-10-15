package tests

import (
	"os"
	"path/filepath"
	"testing"

	"gemini-web2api/internal/config"
	"gemini-web2api/internal/cookies"
	"gemini-web2api/internal/gateway"
	"gemini-web2api/internal/pool"
	"gemini-web2api/internal/storage"
	"gemini-web2api/tests/testutil"
)

func TestMigrateLegacyCookie(t *testing.T) {
	testutil.ResetState(t)
	const raw = "SAPISID=legacy; SID=y"
	mustKV(t, "google_cookie", raw)

	pool.SeedCookiesFromConfig()

	list := pool.AccountList()
	if len(list) != 1 || list[0].Cookie != raw {
		t.Fatalf("没搬进池子: %+v", list)
	}
	// 原值留着：回滚到旧版时那条单 cookie 还能用，行为不变
	if storage.GetValue("google_cookie") != raw {
		t.Errorf("不该动 kv 里的原值")
	}

	// 用户把它从池子里删掉之后，再启动不该复活
	_ = pool.AccountDelete(list[0].ID)
	pool.SeedCookiesFromConfig()
	if n := len(pool.AccountList()); n != 0 {
		t.Errorf("删掉后又被塞回来了，池子里还有 %d 条", n)
	}
}

// 缺 SAPISID 的 cookie 也要照收：旧版把它原样当 Cookie 头发出去，能不能用由上游定。
// 升级不能因为我们新加了校验，就让用户的号悄无声息地退回匿名。
func TestMigrateLegacyCookieWithoutSAPISID(t *testing.T) {
	testutil.ResetState(t)
	const raw = "SID=only; HSID=x; __Secure-1PSID=z"
	mustKV(t, "google_cookie", raw)

	pool.SeedCookiesFromConfig()

	list := pool.AccountList()
	if len(list) != 1 || list[0].Cookie != raw {
		t.Fatalf("缺 SAPISID 就被拦下了，用户升级后会变匿名: %+v", list)
	}
	if !gateway.HasCookie() {
		t.Error("池子里有账号，hasCookie 却是 false")
	}
	// 但手工从面板加同样的值仍然要拦——那时用户当场看得到错误提示
	if _, err := pool.AccountAdd("手工", raw, ""); err == nil {
		t.Error("面板手工添加不该放过缺 SAPISID 的 cookie")
	}
}

// 迁移失败**不能**标记完成，否则用户的 cookie 就永远进不了池子了。
func TestMigrateLegacyCookieBadValueKeepsRetrying(t *testing.T) {
	testutil.ResetState(t)
	// JSON 形式但取不出 cookie 字段：normalizeCookie 判失败
	mustKV(t, "google_cookie", `{"sapisid":"x"}`)

	pool.SeedCookiesFromConfig()

	if n := len(pool.AccountList()); n != 0 {
		t.Fatalf("不该入池，实际 %d 条", n)
	}
	if storage.GetValue("legacy_single_cookie_migrated") == "1" {
		t.Error("入池失败却标记了迁移完成，用户的 cookie 从此再没机会进池子")
	}
	if storage.GetValue("google_cookie") == "" {
		t.Error("入池失败还把原值清了，等于直接丢数据")
	}
}

// scheme 大小写不敏感：url.Parse 会把 scheme 转小写，所以 HTTP:// 在 4.0.0
// 那条不校验的静态代理路径上是能用的，升级时不能被拦下来。
func TestProxySchemeCaseInsensitive(t *testing.T) {
	testutil.ResetState(t)
	mustKV(t, config.RuntimeConfigKey, `{"proxy":"HTTP://1.2.3.4:8080"}`)

	pool.SeedProxiesFromConfig()

	list := pool.ListProxies()
	if len(list) != 1 || list[0].URL != "HTTP://1.2.3.4:8080" {
		t.Fatalf("大写 scheme 被拦了，用户升级后代理不工作: %+v", list)
	}
}

// 缺 scheme 的值确实会被 proxyCreate 拒收，这里保证拒收时不丢原值、不标记完成。
//
// 跟 cookie 那条的处理不同（那边放宽了校验）：`1.2.3.4:8080` 在 4.0.0 上**本来就没生效过**
// —— url.Parse 对它直接报错（first path segment in URL cannot contain colon），
// t.Proxy 保持 nil，请求走的是直连。所以拒收它不构成功能回退，不需要放宽。
func TestMigrateLegacyProxyRejectedKeepsRetrying(t *testing.T) {
	testutil.ResetState(t)
	mustKV(t, config.RuntimeConfigKey, `{"proxy":"1.2.3.4:8080","per_ip_rph":80}`)

	pool.SeedProxiesFromConfig()

	if n := len(pool.ListProxies()); n != 0 {
		t.Fatalf("缺 scheme 的值不该入池，实际 %d 条", n)
	}
	if storage.GetValue("legacy_static_proxy_migrated") == "1" {
		t.Error("入池失败却标记了迁移完成")
	}
	// 原始 runtime_config 一个字都不能动：里面还有别的字段
	if got := storage.GetValue(config.RuntimeConfigKey); got != `{"proxy":"1.2.3.4:8080","per_ip_rph":80}` {
		t.Errorf("runtime_config 被改写了: %s", got)
	}
}

func TestMigrateLegacyProxyOK(t *testing.T) {
	testutil.ResetState(t)
	mustKV(t, config.RuntimeConfigKey, `{"proxy":"http://u:p@1.2.3.4:8080","per_ip_rph":80}`)

	pool.SeedProxiesFromConfig()

	list := pool.ListProxies()
	if len(list) != 1 || list[0].URL != "http://u:p@1.2.3.4:8080" {
		t.Fatalf("没搬进池子: %+v", list)
	}
	// 迁移不改写 runtime_config —— 少动一次已有数据就少一分写坏别的字段的风险
	if got := storage.GetValue(config.RuntimeConfigKey); got != `{"proxy":"http://u:p@1.2.3.4:8080","per_ip_rph":80}` {
		t.Errorf("runtime_config 被改写了: %s", got)
	}
	// 幂等：再启动一次不该多出一条
	pool.SeedProxiesFromConfig()
	if n := len(pool.ListProxies()); n != 1 {
		t.Errorf("第二次启动又加了一条，池子里 %d 条", n)
	}
}

// 改 compose 里的 --proxy 应该是**换掉**那条，不是再加一条。
// 累积的话旧出口会留在池子里继续 enabled、继续接流量，成了僵尸出口。
func TestSeededProxyReplacedOnChange(t *testing.T) {
	testutil.ResetState(t)
	config.Startup().Proxy = "http://old:1080"
	pool.SeedProxiesFromConfig()
	if list := pool.ListProxies(); len(list) != 1 || list[0].URL != "http://old:1080" {
		t.Fatalf("首次播种不对: %+v", list)
	}

	config.Startup().Proxy = "http://new:1080" // 用户改了 compose 后重启
	pool.SeedProxiesFromConfig()

	list := pool.ListProxies()
	if len(list) != 1 {
		t.Fatalf("应替换成 1 条，实际 %d 条: %+v", len(list), list)
	}
	if list[0].URL != "http://new:1080" {
		t.Errorf("换成了 %s，期望 http://new:1080", list[0].URL)
	}

	// 参数被彻底去掉 → 我们建的那条也撤下
	config.Startup().Proxy = ""
	pool.SeedProxiesFromConfig()
	if n := len(pool.ListProxies()); n != 0 {
		t.Errorf("去掉 --proxy 后应撤下，池子里还有 %d 条", n)
	}
}

// 参数没变时完全不碰池子：用户在面板上停用/删除这条记录，说了算。
func TestSeededProxyRespectsPanelEdits(t *testing.T) {
	testutil.ResetState(t)
	config.Startup().Proxy = "http://seed:1080"
	pool.SeedProxiesFromConfig()
	id := pool.ListProxies()[0].ID

	no := false
	if err := pool.ProxyUpdate(id, "", "", &no, nil); err != nil { // 用户在面板停用它
		t.Fatal(err)
	}
	pool.SeedProxiesFromConfig() // 重启

	list := pool.ListProxies()
	if len(list) != 1 {
		t.Fatalf("不该动池子，实际 %d 条", len(list))
	}
	if list[0].Enabled {
		t.Error("用户停用的记录被重新启用了")
	}
}

// 用户自己已经在面板加了同一个出口时，播种不该再建一条重的。
func TestSeededProxySkipsExistingURL(t *testing.T) {
	testutil.ResetState(t)
	if _, err := pool.ProxyCreate("手工加的", "http://same:1080", 1); err != nil {
		t.Fatal(err)
	}
	config.Startup().Proxy = "http://same:1080"
	pool.SeedProxiesFromConfig()
	if n := len(pool.ListProxies()); n != 1 {
		t.Errorf("同一个 URL 被建了 %d 条", n)
	}
}

// --cookie-file 轮换：替换同一条，不能越堆越多。
// 堆积的死 cookie 仍然 enabled、仍然参与轮转，等于每 N 个请求就有一个注定失败。
func TestSeededCookieFileRotates(t *testing.T) {
	testutil.ResetState(t)
	path := filepath.Join(t.TempDir(), "cookie.txt")
	write := func(s string) {
		if err := os.WriteFile(path, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	config.Startup().CookieFile = path

	write("  SAPISID=first; SID=z\n")
	pool.SeedCookiesFromConfig()
	pool.SeedCookiesFromConfig() // 内容没变的第二次启动
	list := pool.AccountList()
	if len(list) != 1 {
		t.Fatalf("内容没变不该重复插，实际 %d 条", len(list))
	}
	if list[0].Cookie != "SAPISID=first; SID=z" {
		t.Errorf("首尾空白没去掉: %q", list[0].Cookie)
	}

	write("SAPISID=rotated; SID=z")
	pool.SeedCookiesFromConfig()
	list = pool.AccountList()
	if len(list) != 1 {
		t.Fatalf("轮换后应仍是 1 条，实际 %d 条: %+v", len(list), list)
	}
	if list[0].Cookie != "SAPISID=rotated; SID=z" {
		t.Errorf("没换成新的: %q", list[0].Cookie)
	}
}

// 文件读不到（容器少挂了个卷之类）时保持现状，不能把正在用的 cookie 撤下来。
func TestSeededCookieFileMissingKeepsPool(t *testing.T) {
	testutil.ResetState(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "cookie.txt")
	if err := os.WriteFile(path, []byte("SAPISID=live; SID=z"), 0o600); err != nil {
		t.Fatal(err)
	}
	config.Startup().CookieFile = path
	pool.SeedCookiesFromConfig()
	if len(pool.AccountList()) != 1 {
		t.Fatal("前置条件不成立")
	}

	_ = os.Remove(path) // 卷没挂上
	pool.SeedCookiesFromConfig()

	if n := len(pool.AccountList()); n != 1 {
		t.Errorf("文件读不到就把 cookie 撤了，池子里剩 %d 条", n)
	}
}

// 旧单 cookie 路径吃 JSON 形式，入池前必须归一化成裸 cookie 串，
// 否则池子里存的是一整段 JSON，SAPISID 提取和后续请求全错。
func TestMigrateLegacyCookieJSONForm(t *testing.T) {
	testutil.ResetState(t)
	mustKV(t, "google_cookie", `{"cookie":"SAPISID=fromjson; SID=w","sapisid":"fromjson"}`)

	pool.SeedCookiesFromConfig()

	list := pool.AccountList()
	if len(list) != 1 {
		t.Fatalf("应有 1 条，实际 %d 条", len(list))
	}
	if list[0].Cookie != "SAPISID=fromjson; SID=w" {
		t.Errorf("JSON 没归一化: %q", list[0].Cookie)
	}
	if got := cookies.SAPISID(list[0].Cookie); got != "fromjson" {
		t.Errorf("入池后取不出 SAPISID: %q", got)
	}
}

// 全新部署：没有任何遗留配置，不该凭空造出账号或代理。
func TestSeedNoop(t *testing.T) {
	testutil.ResetState(t)
	pool.SeedCookiesFromConfig()
	pool.SeedProxiesFromConfig()
	if n := len(pool.AccountList()); n != 0 {
		t.Errorf("不该新建账号，实际 %d 条", n)
	}
	if n := len(pool.ListProxies()); n != 0 {
		t.Errorf("不该新建代理，实际 %d 条", n)
	}
	if gateway.HasCookie() {
		t.Error("池子空时 hasCookie 应为 false")
	}
}

func mustKV(t *testing.T, k, v string) {
	t.Helper()
	if err := storage.SetValue(k, v); err != nil {
		t.Fatal(err)
	}
}
