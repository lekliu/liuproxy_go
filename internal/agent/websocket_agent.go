package agent

import (
	"net"
	"net/http"
)

// WebSocketAgent 实现了 Agent 接口，用于处理 HTTP-over-WebSocket 代理
type WebSocketAgent struct {
	config     Config
	bufferSize int
}

// NewWebSocketAgent 创建一个新的 WebSocketAgent 实例
func NewWebSocketAgent(cfg Config, bufferSize int) *WebSocketAgent {
	return &WebSocketAgent{
		config:     cfg,
		bufferSize: bufferSize,
	}
}

// HandleConnection 是 local 端 WebSocket 代理的入口
func (a *WebSocketAgent) HandleConnection(inboundConn net.Conn) {
	// 这个方法仅由 local 端的 TCP 监听器调用
	if !a.config.IsServerSide {
		a.handleLocal(inboundConn)
	}
}

// HandleUpgrade 是 remote 端 WebSocket 代理的入口，由 HTTP server 调用
func (a *WebSocketAgent) HandleUpgrade(w http.ResponseWriter, r *http.Request) {
	if a.config.IsServerSide {
		a.handleRemote(w, r)
	}
}
