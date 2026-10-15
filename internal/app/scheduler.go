package app

import (
	"time"

	"gemini-web2api/internal/gateway"
	"gemini-web2api/internal/gemini"
	"gemini-web2api/internal/pool"
	"gemini-web2api/internal/storage"
)

// startScheduler kicks off background loops:
//  1. hourly aggregation: rolls up requests in the previous closed hour bucket
//  2. daily aggregation: rolls up the previous closed day bucket
//  3. retention: deletes raw requests older than rtCfg().RetentionDays
//  4. proxy cache reload: every 60s pull proxies from DB
func startScheduler() {
	go func() {
		// On boot: catch up any aggregations missed while service was down.
		storage.AggregateHourlyCatchup()
		storage.AggregateDailyCatchup()
		storage.RetentionSweep()

		hourly := time.NewTicker(5 * time.Minute)
		daily := time.NewTicker(1 * time.Hour)
		retention := time.NewTicker(6 * time.Hour)
		proxyReload := time.NewTicker(60 * time.Second)
		// 会话保活：启动后先尽快刷一次 1PSIDTS（导入的票可能已经快到期），
		// 之后按服务端在轮转页里指定的间隔走（实测 600 秒）。
		rotate := time.NewTimer(gemini.FirstRotateDelay)
		defer hourly.Stop()
		defer daily.Stop()
		defer retention.Stop()
		defer proxyReload.Stop()
		defer rotate.Stop()

		for {
			select {
			case <-hourly.C:
				storage.AggregateHourlyCatchup()
			case <-daily.C:
				storage.AggregateDailyCatchup()
			case <-retention.C:
				storage.RetentionSweep()
			case <-proxyReload.C:
				pool.LoadProxies()
			case <-rotate.C:
				rotate.Reset(gateway.RotateAllAccounts())
			}
		}
	}()
}
