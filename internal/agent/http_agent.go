package agent

import "net"

type HTTPAgent struct {
	config     Config
	bufferSize int
}

func NewHTTPAgent(cfg Config, bufferSize int) *HTTPAgent {
	return &HTTPAgent{config: cfg, bufferSize: bufferSize}
}

// HandleConnection 是 Agent 的入口点
func (a *HTTPAgent) HandleConnection(inboundConn net.Conn) {
	a.handle(inboundConn)
}
