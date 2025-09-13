// --- START OF COMPLETE REPLACEMENT for tunnel_strategy.go (REVERTED) ---
package socks5

import (
	"bufio"
	"fmt"
	"liuproxy_go/internal/types"
	"log"
	"net"
)

// TunnelStrategy 定义了处理客户端代理请求的通用接口。
// 它是连接分发器（dispatcher）和具体后端实现（Go Remote, Worker）之间的桥梁。
type TunnelStrategy interface {
	Initialize() error
	HandleConnection(conn net.Conn, reader *bufio.Reader)
	GetType() string
	// Add new methods for dynamic reloading
	CloseTunnel()
	UpdateServers(profiles []*BackendProfile)
	GetTunnelManager() *TunnelManager
}

// BackendProfile 结构体用于存储从配置文件中解析出的单个后端配置。
type BackendProfile struct {
	Type    string
	Remarks string
	Address string
	Port    string
	Scheme  string
	Path    string
	EdgeIP  string
	Active  bool
}

// CreateStrategyFromConfig 是一个工厂函数，根据配置创建并返回相应的策略实例。
func CreateStrategyFromConfig(cfg *types.Config, profiles []*BackendProfile) (TunnelStrategy, error) {
	log.Println("--- [StrategyFactory] DIAGNOSIS ---")
	log.Printf("CreateStrategyFromConfig called with %d total profiles:", len(profiles))

	var activeProfile *BackendProfile
	for _, p := range profiles {
		if p.Active {
			activeProfile = p
			break // 找到第一个激活的就停止
		}
	}

	// 如果没有找到任何激活的配置，则返回错误
	if activeProfile == nil {
		if len(profiles) > 0 {
			// 如果有配置但没有一个被激活，这是个错误状态
			return nil, fmt.Errorf("no active backend profile found in configuration")
		}
		// 如果连配置都没有，返回更明确的错误
		return nil, fmt.Errorf("no backend profiles configured or enabled")
	}

	log.Printf("  >>> Active Profile Found: Type=%s, Remarks=%s, Address=%s", activeProfile.Type, activeProfile.Remarks, activeProfile.Address)

	// --- 关键修改: 现在总是使用 activeProfile ---
	switch activeProfile.Type {
	case "worker":
		// Worker 策略只关心它自己的配置
		return NewWorkerStrategy(cfg, activeProfile)
	case "remote":
		fallthrough
	default:
		// GoRemote 策略现在也只接收它自己的配置
		// 我们将其包装成一个只含有一个元素的切片来传递
		return NewGoRemoteStrategy(cfg, []*BackendProfile{activeProfile})
	}
}

// --- END OF COMPLETE REPLACEMENT ---
