package types

import (
	"context"
	"net"
)

// TunnelStrategy 定义了所有策略的通用接口。
type TunnelStrategy interface {
	Initialize() error
	GetType() string
	CloseTunnel()
	GetListenerInfo() *ListenerInfo
	GetMetrics() *Metrics
	UpdateServer(profile *ServerProfile) error
	CheckHealth() error
}

// ServerState 封装了与单个服务器相关的所有信息：配置、实例和运行时状态。
// 这是系统中代表一个服务器通道的唯一事实来源。
type ServerState struct {
	Profile  *ServerProfile // 静态配置 (来自 servers.json)
	Instance TunnelStrategy // 动态的策略实例 (可能为 nil)

	// -- 运行时状态字段 --
	Health  HealthStatus // 健康检查状态 (Up/Down/Unknown)
	Metrics *Metrics     // 性能指标 (连接数, 延迟)
}

// StateProvider 接口定义了一个提供实时后端状态的查询器。
// AppServer 将实现此接口，并将其注入 Dispatcher。
type StateProvider interface {
	GetServerStates() map[string]*ServerState
}

// FailureReporter defines an interface for reporting connection status.
// This decouples AppServer from the full Dispatcher interface.
type FailureReporter interface {
	ReportFailure(serverID string)
	ReportSuccess(serverID string)
}

// Dispatcher 接口定义了路由决策器的核心功能。
type Dispatcher interface {
	// Dispatch 接收源地址和目标地址，返回一个选择的后端实例的监听地址。
	Dispatch(ctx context.Context, source net.Addr, target string) (string, string, error)
}

type HealthStatus int

const (
	StatusUnknown HealthStatus = iota // Default value
	StatusUp
	StatusDown
)
