package agent

import "net"

// Agent 是所有代理类型的通用接口
type Agent interface {
	// HandleConnection 处理一个入站连接
	HandleConnection(inboundConn net.Conn)
}

// Config 是所有 Agent 配置的通用接口
type Config struct {
	IsServerSide bool
	EncryptKey   int
	RemoteHost   string
	RemotePort   int
	RemoteScheme string // Add this line for WebSocket scheme ("ws" or "wss")
}
