// --- START OF COMPLETE REPLACEMENT for tunnel_strategy.go ---
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
}

// CreateStrategyFromConfig 是一个工厂函数，根据配置创建并返回相应的策略实例。
func CreateStrategyFromConfig(cfg *types.Config, profiles []*BackendProfile) (TunnelStrategy, error) {
	log.Println("--- [StrategyFactory] DIAGNOSIS ---")
	log.Printf("CreateStrategyFromConfig called with %d profiles:", len(profiles))
	for i, p := range profiles {
		log.Printf("  Profile[%d]: Type=%s, Remarks=%s, Address=%s", i, p.Type, p.Remarks, p.Address)
	}

	if len(profiles) == 0 {
		return nil, fmt.Errorf("no backend profiles configured or enabled")
	}

	profile := profiles[0]

	switch profile.Type {
	case "worker":
		return NewWorkerStrategy(cfg, profile)
	case "remote":
		fallthrough
	default:
		return NewGoRemoteStrategy(cfg, profiles)
	}
}

// --- END OF COMPLETE REPLACEMENT ---
