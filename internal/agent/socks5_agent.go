package agent

import "net"

// Socks5Agent 实现了 Agent 接口，用于处理 SOCKS5 代理逻辑
type Socks5Agent struct {
	config     Config
	bufferSize int
	addr       net.Addr // 仅 local 端需要，用于响应
}

// NewSocks5Agent 创建一个新的 Socks5Agent 实例
func NewSocks5Agent(cfg Config, bufferSize int, addr net.Addr) *Socks5Agent {
	return &Socks5Agent{
		config:     cfg,
		bufferSize: bufferSize,
		addr:       addr,
	}
}

// HandleConnection 是 SOCKS5 代理的入口
func (a *Socks5Agent) HandleConnection(inboundConn net.Conn) {
	a.handle(inboundConn)
}
