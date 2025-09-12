// --- START OF COMPLETE REPLACEMENT for remote_strategy.go ---
package socks5

import (
	"bufio"
	"fmt"
	"liuproxy_go/internal/types"
	"net"
)

// GoRemoteStrategy 实现了与Go Remote后端通信的策略。
// 它内部封装了TunnelManager和协议处理器，以支持多路复用。
type GoRemoteStrategy struct {
	config        *types.Config
	tunnelManager *TunnelManager
	udpManager    *UDPManager
	httpHandler   *HTTPHandler
	socks5Handler *Agent
}

// NewGoRemoteStrategy 创建一个新的GoRemoteStrategy实例。
func NewGoRemoteStrategy(cfg *types.Config, profiles []*BackendProfile) (TunnelStrategy, error) {
	// 1. 将profiles转换为TunnelManager所需的字符串数组格式
	var serverStrings [][]string
	for _, p := range profiles {
		// GoRemoteStrategy只处理 'remote' 类型的后端
		if p.Type == "remote" {
			// remoteCfg格式: host, port, scheme, path, type, edge_ip
			// (type和edge_ip对GoRemoteStrategy的旧逻辑是可选的，但为了统一格式而保留)
			serverStrings = append(serverStrings, []string{p.Address, p.Port, p.Scheme, p.Path, "remote", ""})
		}
	}
	if len(serverStrings) == 0 {
		return nil, fmt.Errorf("no 'remote' type backends found for GoRemoteStrategy")
	}

	// 2. 创建并组装原有的核心组件 (逻辑平移自旧的local_server.go)
	agentForManager := NewAgent(cfg, cfg.CommonConf.BufferSize).(*Agent)
	tunnelManager := NewTunnelManager(agentForManager, serverStrings)
	udpManager := NewUDPManager(tunnelManager, cfg.CommonConf.BufferSize)
	agentForManager.SetLocalUDPManager(udpManager)

	socks5Handler := NewAgent(cfg, cfg.CommonConf.BufferSize).(*Agent)
	socks5Handler.SetTunnelManager(tunnelManager)
	socks5Handler.SetLocalUDPManager(udpManager)

	httpHandler := NewHTTPHandler(tunnelManager)

	return &GoRemoteStrategy{
		config:        cfg,
		tunnelManager: tunnelManager,
		udpManager:    udpManager,
		httpHandler:   httpHandler,
		socks5Handler: socks5Handler,
	}, nil
}

// Initialize 负责启动GoRemote策略所需的后台任务，例如预连接隧道。
func (s *GoRemoteStrategy) Initialize() error {
	_, err := s.tunnelManager.GetConnection()
	if err != nil {
		return err // Return the error
	}
	return nil // Return nil on success
}

// HandleConnection 实现了协议嗅探和分发，逻辑平移自旧的local_server.go
func (s *GoRemoteStrategy) HandleConnection(conn net.Conn, reader *bufio.Reader) {
	firstByte, err := reader.Peek(1)
	if err != nil {
		return
	}

	if firstByte[0] == 0x05 {
		s.socks5Handler.HandleConnection(conn, reader)
	} else {
		s.httpHandler.HandleConnection(conn, reader)
	}
}

// GetType 返回策略的类型。
func (s *GoRemoteStrategy) GetType() string {
	return "remote"
}

// CloseTunnel allows the AppServer to trigger a disconnection of the current tunnel.
func (s *GoRemoteStrategy) CloseTunnel() {
	if s.tunnelManager != nil {
		s.tunnelManager.CloseCurrentConnection()
	}
}

// UpdateServers allows the AppServer to update the list of remote backends.
func (s *GoRemoteStrategy) UpdateServers(profiles []*BackendProfile) {
	if s.tunnelManager != nil {
		var serverStrings [][]string
		for _, p := range profiles {
			if p.Type == "remote" {
				serverStrings = append(serverStrings, []string{p.Address, p.Port, p.Scheme, p.Path, "remote", ""})
			}
		}
		s.tunnelManager.UpdateRemoteServers(serverStrings)
	}
}

// GetTunnelManager exposes the internal TunnelManager.
func (s *GoRemoteStrategy) GetTunnelManager() *TunnelManager {
	return s.tunnelManager
}
