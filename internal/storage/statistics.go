package storage

import (
	"database/sql"
	"fmt"
	"sort"
	"time"

	"gemrelay/internal/config"
)

// AggregateHourlyCatchup walks every hour bucket from the latest aggregated
// hour up to (now - 1h), so a long downtime catches up cleanly.
func AggregateHourlyCatchup() {
	var lastBucket int64
	if err := DB().QueryRow(`SELECT COALESCE(MAX(bucket), 0) FROM stats_hourly`).Scan(&lastBucket); err != nil {
		config.Logf("[agg] 读取小时聚合进度失败: %v", err)
		return
	}
	now := time.Now().Unix()
	currentHourStart := now - (now % 3600)
	target := currentHourStart - 3600 // most recent CLOSED hour
	if lastBucket == 0 {
		// First run — bootstrap from oldest request available.
		var minTS int64
		if err := DB().QueryRow(`SELECT COALESCE(MIN(ts), 0) FROM requests`).Scan(&minTS); err != nil {
			config.Logf("[agg] 读取最早请求时间失败: %v", err)
			return
		}
		if minTS == 0 {
			return
		}
		lastBucket = minTS - (minTS % 3600) - 3600
	}
	for b := lastBucket + 3600; b <= target; b += 3600 {
		if err := AggregateBucket(b, 3600, "stats_hourly"); err != nil {
			config.Logf("[agg] 小时聚合失败，停止推进以便下次重试: %v", err)
			return
		}
	}
}

func AggregateDailyCatchup() {
	var lastBucket int64
	if err := DB().QueryRow(`SELECT COALESCE(MAX(bucket), 0) FROM stats_daily`).Scan(&lastBucket); err != nil {
		config.Logf("[agg] 读取日聚合进度失败: %v", err)
		return
	}
	now := time.Now().Unix()
	currentDayStart := now - (now % 86400)
	target := currentDayStart - 86400
	if lastBucket == 0 {
		var minTS int64
		if err := DB().QueryRow(`SELECT COALESCE(MIN(bucket), 0) FROM stats_hourly`).Scan(&minTS); err != nil {
			config.Logf("[agg] 读取最早小时聚合时间失败: %v", err)
			return
		}
		if minTS == 0 {
			return
		}
		lastBucket = minTS - (minTS % 86400) - 86400
	}
	for b := lastBucket + 86400; b <= target; b += 86400 {
		if err := AggregateBucket(b, 86400, "stats_daily"); err != nil {
			config.Logf("[agg] 日聚合失败，停止推进以便下次重试: %v", err)
			return
		}
	}
}

// AggregateBucket reads raw requests in [start, start+span) and groups by
// (model, proxy_id), writing one row per group.
// Source for hourly = requests; for daily = stats_hourly (already aggregated).
func AggregateBucket(start, span int64, target string) error {
	end := start + span

	// We pre-aggregate latencies in Go to compute p50/p95 (SQLite percentile is awkward).
	type key struct {
		model string
		proxy int64
	}
	type acc struct {
		count, success, fail   int
		totalMs, promptT, outT int64
		latencies              []int
	}
	groups := map[key]*acc{}

	var rows *sql.Rows
	var err error

	if target == "stats_hourly" {
		// from raw requests
		r, e := DB().Query(`SELECT model, COALESCE(proxy_id,0), status, total_ms, prompt_tokens, output_tokens
			FROM requests WHERE ts >= ? AND ts < ?`, start, end)
		err = e
		rows = r
	} else {
		// from hourly aggregates
		r, e := DB().Query(`SELECT model, proxy_id, requests, successes, failures, total_ms, prompt_tokens, output_tokens, p50_ms, p95_ms
			FROM stats_hourly WHERE bucket >= ? AND bucket < ?`, start, end)
		err = e
		rows = r
	}
	if err != nil {
		return fmt.Errorf("%s 查询失败: %w", target, err)
	}
	defer rows.Close()

	if target == "stats_hourly" {
		for rows.Next() {
			var model string
			var proxyID int64
			var status int
			var totalMs, pt, ot int64
			if err := rows.Scan(&model, &proxyID, &status, &totalMs, &pt, &ot); err != nil {
				return fmt.Errorf("%s 读取请求失败: %w", target, err)
			}
			k := key{model, proxyID}
			a := groups[k]
			if a == nil {
				a = &acc{}
				groups[k] = a
			}
			a.count++
			if status == 200 {
				a.success++
			} else {
				a.fail++
			}
			a.totalMs += totalMs
			a.promptT += pt
			a.outT += ot
			a.latencies = append(a.latencies, int(totalMs))
		}
	} else {
		// daily: roll up hourly rows. p50/p95 = avg of hourly p50/p95 (good enough).
		type dailyAcc struct {
			acc
			p50Sum, p95Sum, p50N, p95N int
		}
		dgroups := map[key]*dailyAcc{}
		for rows.Next() {
			var model string
			var proxyID int64
			var requests, successes, failures int
			var totalMs, pt, ot int64
			var p50, p95 int
			if err := rows.Scan(&model, &proxyID, &requests, &successes, &failures, &totalMs, &pt, &ot, &p50, &p95); err != nil {
				return fmt.Errorf("%s 读取小时统计失败: %w", target, err)
			}
			k := key{model, proxyID}
			a := dgroups[k]
			if a == nil {
				a = &dailyAcc{}
				dgroups[k] = a
			}
			a.count += requests
			a.success += successes
			a.fail += failures
			a.totalMs += totalMs
			a.promptT += pt
			a.outT += ot
			if p50 > 0 {
				a.p50Sum += p50
				a.p50N++
			}
			if p95 > 0 {
				a.p95Sum += p95
				a.p95N++
			}
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("%s 遍历失败: %w", target, err)
		}
		if err := rows.Close(); err != nil {
			return fmt.Errorf("%s 关闭查询失败: %w", target, err)
		}
		tx, err := DB().Begin()
		if err != nil {
			return fmt.Errorf("%s 开始事务失败: %w", target, err)
		}
		defer tx.Rollback()
		for k, a := range dgroups {
			p50 := 0
			if a.p50N > 0 {
				p50 = a.p50Sum / a.p50N
			}
			p95 := 0
			if a.p95N > 0 {
				p95 = a.p95Sum / a.p95N
			}
			_, err := tx.Exec(upsert("stats_daily",
				[]string{"bucket", "model", "proxy_id", "requests", "successes", "failures",
					"total_ms", "p50_ms", "p95_ms", "prompt_tokens", "output_tokens"},
				[]string{"bucket", "model", "proxy_id"}),
				start, k.model, k.proxy,
				a.count, a.success, a.fail, a.totalMs, p50, p95, a.promptT, a.outT)
			if err != nil {
				return fmt.Errorf("%s 写入失败: %w", target, err)
			}
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("%s 提交失败: %w", target, err)
		}
		return nil
	}

	// hourly bucket commit
	if err := rows.Err(); err != nil {
		return fmt.Errorf("%s 遍历失败: %w", target, err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("%s 关闭查询失败: %w", target, err)
	}
	tx, err := DB().Begin()
	if err != nil {
		return fmt.Errorf("%s 开始事务失败: %w", target, err)
	}
	defer tx.Rollback()
	for k, a := range groups {
		p50, p95 := percentiles(a.latencies)
		_, err := tx.Exec(upsert("stats_hourly",
			[]string{"bucket", "model", "proxy_id", "requests", "successes", "failures",
				"total_ms", "p50_ms", "p95_ms", "prompt_tokens", "output_tokens"},
			[]string{"bucket", "model", "proxy_id"}),
			start, k.model, k.proxy,
			a.count, a.success, a.fail, a.totalMs, p50, p95, a.promptT, a.outT)
		if err != nil {
			return fmt.Errorf("%s 写入失败: %w", target, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%s 提交失败: %w", target, err)
	}
	return nil
}

func percentiles(latencies []int) (p50, p95 int) {
	if len(latencies) == 0 {
		return 0, 0
	}
	sort.Ints(latencies)
	p50 = latencies[len(latencies)*50/100]
	p95idx := len(latencies) * 95 / 100
	if p95idx >= len(latencies) {
		p95idx = len(latencies) - 1
	}
	p95 = latencies[p95idx]
	return
}

// RetentionSweep deletes raw requests older than RetentionDays and expired sessions.
func RetentionSweep() {
	if config.Runtime().RetentionDays <= 0 {
		return
	}
	cutoff := time.Now().Unix() - int64(config.Runtime().RetentionDays)*86400
	res, err := DB().Exec(`DELETE FROM requests WHERE ts < ?`, cutoff)
	if err != nil {
		config.Logf("[retention] failed: %v", err)
		return
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		config.Logf("[retention] purged %d old request rows", n)
	}
	if _, err := DB().Exec(`DELETE FROM sessions WHERE expires_at < ?`, time.Now().Unix()); err != nil {
		config.Logf("[retention] 清理过期会话失败: %v", err)
	}
}
