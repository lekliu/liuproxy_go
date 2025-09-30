package mobile

import (
	"encoding/json"
	"fmt"
	"liuproxy_go/internal/shared/logger"
	"liuproxy_go/internal/tunnel"
	"sync"

	"liuproxy_go/internal/shared/types"
)

var (
	// 全局变量，用于持有当前为移动端运行的唯一策略实例
	activeStrategy types.TunnelStrategy
	instanceMutex  sync.Mutex
)

// StartVPN 启动 Go 核心的 local 代理逻辑。
// 它接收一个服务器配置的JSON字符串，动态创建一个SOCKS5/HTTP统一监听器，并返回其监听的端口号。
func StartVPN(profileJSON string) (int, error) {
	instanceMutex.Lock()
	defer instanceMutex.Unlock()

	if activeStrategy != nil {
		return 0, fmt.Errorf("service is already running")
	}

	// 1. 解析传入的 ServerProfile JSON
	var profile types.ServerProfile
	if err := json.Unmarshal([]byte(profileJSON), &profile); err != nil {
		return 0, fmt.Errorf("failed to parse profile JSON: %w", err)
	}

	// 2. 强制设置profile为激活状态，并使用动态端口
	profile.Active = true
	profile.LocalPort = 0

	// 3. 构建一个最小化的配置，仅包含移动端必要的通用设置
	cfg := &types.Config{
		CommonConf: types.CommonConf{
			Mode:           "local",
			MaxConnections: 32,   // Mobile specific value
			BufferSize:     4096, // Mobile specific value
			Crypt:          125,  // TODO: Should be part of the profile
		},
		LogConf: types.LogConf{
			Level: "debug", // Mobile defaults to debug log for better troubleshooting
		},
	}

	// 4. 初始化日志系统
	if err := logger.Init(cfg.LogConf); err != nil {
		// 在日志系统失败时，回退到标准日志库
		fmt.Printf("Fatal: Failed to initialize logger: %v\n", err)
		return 0, fmt.Errorf("failed to initialize logger: %w", err)
	}

	logger.Info().Msg("Configuring and starting Go core for mobile...")

	// 5. 使用策略工厂创建一个新的策略实例
	newStrategy, err := tunnel.NewStrategy(cfg, []*types.ServerProfile{&profile})
	if err != nil {
		logger.Error().Err(err).Msg("Failed to create new strategy for mobile")
		return 0, fmt.Errorf("failed to create strategy: %w", err)
	}

	// 6. 初始化策略实例，这将启动其内部监听器
	if err := newStrategy.Initialize(); err != nil {
		logger.Error().Err(err).Msg("Failed to initialize strategy for mobile")
		return 0, fmt.Errorf("failed to initialize strategy: %w", err)
	}

	// 7. 获取动态分配的端口信息
	listenerInfo := newStrategy.GetListenerInfo()
	if listenerInfo == nil || listenerInfo.Port == 0 {
		newStrategy.CloseTunnel()
		return 0, fmt.Errorf("strategy failed to start listener or return a valid port")
	}

	// 8. 保存实例引用并返回成功
	activeStrategy = newStrategy
	logger.Info().Int("port", listenerInfo.Port).Msgf("Go core started successfully, listening on port %d", listenerInfo.Port)

	return listenerInfo.Port, nil
}

// StopVPN 停止 Go 核心。
func StopVPN() {
	instanceMutex.Lock()
	defer instanceMutex.Unlock()

	if activeStrategy != nil {
		logger.Info().Msg("Stopping Go core for mobile...")
		activeStrategy.CloseTunnel()
		activeStrategy = nil
		logger.Info().Msg("Go core stopped.")
	}
}

// GetStats (可选) 未来可以添加一个API用于从App查询流量统计
// func GetStats() string {
//     instanceMutex.Lock()
//     defer instanceMutex.Unlock()
//     if activeStrategy != nil {
//         metrics := activeStrategy.GetMetrics()
//         // format and return metrics as JSON string
//     }
//     return "{}"
// }
