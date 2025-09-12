// --- START OF MODIFIED FILE liuproxy_go/internal/agent/socks5/agent.go ---
package socks5

import (
	"bufio"
	"liuproxy_go/internal/types"
	"net"
)

type Agent struct {
	config *types.Config

	bufferSize     int
	remoteUdpRelay *RemoteUDPRelay
	tunnelManager  *TunnelManager
	udpManager     *UDPManager
}

func NewAgent(cfg *types.Config, bufferSize int) types.Agent {
	return &Agent{
		config:     cfg,
		bufferSize: bufferSize,
	}
}
func (a *Agent) HandleConnection(conn net.Conn, reader *bufio.Reader) {
	defer conn.Close()
	// --- 关键逻辑：不再需要 IsServerSide 字段 ---
	// 远程或本地的行为由不同的策略 (Strategy) 决定，Agent 只负责 SOCKS5 握手
	// 我们假定这个 Agent 实例总是在 Local 端使用，因为它由 GoRemoteStrategy 创建
	if a.config.Mode == "remote" { // 虽然现在总是在 local 端，但为了逻辑完整性保留
		a.handleRemote(conn)
	} else {
		a.handleLocal(conn, reader)
	}
}
func (a *Agent) SetTunnelManager(tm *TunnelManager) {
	a.tunnelManager = tm
}
func (a *Agent) SetUDPRelay(relay *RemoteUDPRelay) {
	a.remoteUdpRelay = relay
}
func (a *Agent) SetLocalUDPManager(manager *UDPManager) {
	a.udpManager = manager
}
