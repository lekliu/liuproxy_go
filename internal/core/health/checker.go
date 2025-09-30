package health

import (
	"liuproxy_go/internal/shared/logger"
	"liuproxy_go/internal/shared/types"
	"sync"
	"time"
)

// Checker 负责对策略实例进行健康检查。
type Checker struct{}

// New 创建一个新的 Checker 实例。
func New() *Checker {
	return &Checker{}
}

// Check 对传入的策略实例 map 进行并发健康检查。
// 它返回健康状态 map 和性能指标 map。
func (c *Checker) Check(instancesToCheck map[string]types.TunnelStrategy) (map[string]types.HealthStatus, map[string]*types.Metrics) {
	healthStatus := make(map[string]types.HealthStatus)
	metricsCache := make(map[string]*types.Metrics)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for id, strat := range instancesToCheck {
		wg.Add(1)
		go func(serverID string, st types.TunnelStrategy) {
			defer wg.Done()

			// 1. 先获取 metrics 对象，并设置默认延迟为 -1
			metrics := st.GetMetrics()
			metrics.Latency = -1

			var currentHealth types.HealthStatus

			// 2. 检查策略实例是否已初始化并开始监听
			listenerInfo := st.GetListenerInfo()
			logFields := logger.Debug().Str("server_id", serverID).Str("strategy_type", st.GetType())

			if listenerInfo == nil || listenerInfo.Port == 0 {
				currentHealth = types.StatusDown
				logFields.Msg("HealthCheck: Instance listener is down or nil.")
			} else {
				// 3. 调用策略自身的端到端健康检查方法，并测量延迟
				start := time.Now()
				err := st.CheckHealth()
				latency := time.Since(start)

				if err == nil {
					currentHealth = types.StatusUp
					metrics.Latency = latency.Milliseconds()
					logFields.Bool("success", true).Int64("latency_ms", metrics.Latency).Msg("HealthCheck: Check passed.")
				} else {
					currentHealth = types.StatusDown
					logFields.Bool("success", false).Err(err).Msg("HealthCheck: Check failed.")
				}
			}

			mu.Lock()
			healthStatus[serverID] = currentHealth
			// 5. 存储更新后的 metrics 对象
			metricsCache[serverID] = metrics
			mu.Unlock()
		}(id, strat)
	}

	wg.Wait()
	return healthStatus, metricsCache
}
