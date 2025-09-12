// --- START OF COMPLETE REPLACEMENT for http_handler.go ---
package socks5

import (
	"bufio"
	"net"
	"strconv"
)

// HTTPHandler 负责处理被调度器识别为 HTTP/HTTPS 的连接
type HTTPHandler struct {
	tunnelManager *TunnelManager
}

// NewHTTPHandler 创建一个新的 HTTP 代理处理器
func NewHTTPHandler(tm *TunnelManager) *HTTPHandler {
	return &HTTPHandler{tunnelManager: tm}
}

// HandleConnection 是 HTTP 代理的入口
func (h *HTTPHandler) HandleConnection(inboundConn net.Conn, reader *bufio.Reader) {
	defer inboundConn.Close()

	// 1. 从已有的 reader 中解析出目标地址和原始请求数据
	initialData, hostname, port, isSSL, err := GetTargetFromHTTPHeader(reader)
	if err != nil {
		return
	}
	targetAddr := net.JoinHostPort(hostname, strconv.Itoa(port))

	// 2. 使用 TunnelManager 创建一个新的会话 (复用 SOCKS5 的会话逻辑)
	//    我们传递 initialData，以便会话可以将它作为第一个上行数据包发送
	session := h.tunnelManager.SessionManager.NewTCPSession(inboundConn, targetAddr, initialData, isSSL)
	if session == nil {
		return
	}

	// 3. 等待会话结束
	session.Wait()
}

// --- END OF COMPLETE REPLACEMENT ---
