package app

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestAccountIDIsNotReused(t *testing.T) {
	first, err := accountAdd("first", "SAPISID=first; SID=x", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := accountDelete(first); err != nil {
		t.Fatal(err)
	}
	second, err := accountAdd("second", "SAPISID=second; SID=x", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = accountDelete(second) })
	if second <= first {
		t.Fatalf("新账号必须使用新身份: first=%d second=%d", first, second)
	}
}

func TestSQLiteConcurrentWrites(t *testing.T) {
	db := getDB()
	if curDialect != dialectSQLite {
		t.Skip("SQLite 连接排队回归")
	}
	if got := db.raw.Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("SQLite 应使用单连接避免进程内写锁争用, got %d", got)
	}
	const key = "test_concurrent_writes"
	if err := kvSet(key, "0"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = getDB().Exec(`DELETE FROM kv WHERE k=?`, key) })
	const workers = 24
	var wg sync.WaitGroup
	errors := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := getDB().Exec(`UPDATE kv SET v=CAST(v AS INTEGER)+1 WHERE k=?`, key)
			errors <- err
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Error(err)
		}
	}
	if got := kvGet(key); got != "24" {
		t.Fatalf("并发写入不能丢失更新, got %s", got)
	}
}

// 在 TestMain 的临时 SQLite 库中使用真实约束失败，验证错误处理，不替换数据库函数。
func TestAccountPickRejectsFailedWrite(t *testing.T) {
	resetSeedState(t)
	id, err := accountAdd("pick", "SAPISID=dummy; SID=x", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = getDB().Exec(`CREATE TRIGGER test_reject_account_pick BEFORE UPDATE OF last_used_at ON accounts
		BEGIN SELECT RAISE(ABORT, 'test write rejected'); END`)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = getDB().Exec(`DROP TRIGGER test_reject_account_pick`) })
	if picked, ok := pickCookieAccount(); ok || picked != nil {
		t.Fatal("使用时间未能保存时不能假装挑号成功")
	}
	if a := accountByID(id); a == nil || a.LastUsedAt != 0 {
		t.Fatal("失败写入不应改变账号使用时间")
	}
}

func TestProxyCacheFollowsSuccessfulWrite(t *testing.T) {
	resetSeedState(t)
	id, err := proxyCreate("write-test", "http://127.0.0.1:1", 1)
	if err != nil {
		t.Fatal(err)
	}
	_, err = getDB().Exec(`CREATE TRIGGER test_reject_proxy_write BEFORE UPDATE ON proxies
		BEGIN SELECT RAISE(ABORT, 'test write rejected'); END`)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = getDB().Exec(`DROP TRIGGER test_reject_proxy_write`) })
	recordProxyResult(id, false, "unreachable")
	for _, proxy := range listProxies() {
		if proxy.ID == id && proxy.FailCount != 0 {
			t.Fatal("数据库拒绝写入时不能单独改变内存健康度")
		}
	}
	var failures int
	if err := getDB().QueryRow(`SELECT fail_count FROM proxies WHERE id=?`, id).Scan(&failures); err != nil || failures != 0 {
		t.Fatalf("数据库状态不应变化: failures=%d err=%v", failures, err)
	}
}

func TestAggregateWritesAreAtomic(t *testing.T) {
	const bucket int64 = 946684800
	for _, target := range []string{"stats_hourly", "stats_daily"} {
		for _, reject := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reject=%v", target, reject), func(t *testing.T) {
				cleanup := func() {
					_, _ = getDB().Exec(`DROP TRIGGER IF EXISTS test_reject_aggregate`)
					_, _ = getDB().Exec(`DELETE FROM requests WHERE model IN (?,?)`, "write-ok", "write-rejected")
					for _, table := range []string{"stats_hourly", "stats_daily"} {
						_, _ = getDB().Exec(`DELETE FROM `+table+` WHERE bucket=? AND model IN (?,?)`, bucket, "write-ok", "write-rejected")
					}
				}
				cleanup()
				t.Cleanup(cleanup)
				span := int64(3600)
				for _, model := range []string{"write-ok", "write-rejected"} {
					var err error
					if target == "stats_hourly" {
						_, err = getDB().Exec(`INSERT INTO requests(ts,model,status,total_ms,prompt_chars,response_chars,prompt_tokens,output_tokens)
							VALUES (?,?,200,20,1,1,1,1)`, bucket, model)
					} else {
						span = 86400
						_, err = getDB().Exec(`INSERT INTO stats_hourly(bucket,model,proxy_id,requests,successes,total_ms,prompt_tokens,output_tokens)
							VALUES (?,?,0,1,1,20,1,1)`, bucket, model)
					}
					if err != nil {
						t.Fatal(err)
					}
				}
				if reject {
					_, err := getDB().Exec(`CREATE TRIGGER test_reject_aggregate BEFORE INSERT ON ` + target + `
						WHEN NEW.model='write-rejected' BEGIN SELECT RAISE(ABORT, 'aggregate constraint failure'); END`)
					if err != nil {
						t.Fatal(err)
					}
				}
				err := aggregateBucket(bucket, span, target)
				if reject {
					if err == nil || !strings.Contains(err.Error(), "aggregate constraint failure") {
						t.Fatalf("事务失败应返回原始约束错误, got %v", err)
					}
				} else if err != nil {
					t.Fatal(err)
				}
				var count int
				if err := getDB().QueryRow(`SELECT COUNT(*) FROM `+target+` WHERE bucket=?`, bucket).Scan(&count); err != nil {
					t.Fatal(err)
				}
				want := 2
				if reject {
					want = 0
				}
				if count != want {
					t.Fatalf("聚合应完整提交或全部回滚: count=%d want %d", count, want)
				}
			})
		}
	}
}
