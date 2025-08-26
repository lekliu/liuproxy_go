package agent

import (
	"io"
	"net"
	"strconv"
	"sync"
)

// DirectAgent 负责处理直连的逻辑
type DirectAgent struct {
	hostname    string
	port        int
	isSSL       bool
	initialData []byte // 浏览器发来的初始请求数据
	bufferSize  int
}

// NewDirectAgent 创建一个新的直连 Agent
func NewDirectAgent(hostname string, port int, isSSL bool, initialData []byte, bufferSize int) *DirectAgent {
	return &DirectAgent{
		hostname:    hostname,
		port:        port,
		isSSL:       isSSL,
		initialData: initialData,
		bufferSize:  bufferSize,
	}
}

// HandleConnection 是直连代理的入口
func (a *DirectAgent) HandleConnection(inboundConn net.Conn) {
	defer inboundConn.Close()

	// 1. 连接到最终目标网站
	outboundConn, err := net.Dial("tcp", net.JoinHostPort(a.hostname, strconv.Itoa(a.port)))
	if err != nil {
		return
	}
	defer outboundConn.Close()

	// 2. 处理初始请求/响应
	if a.isSSL {
		// 如果是 CONNECT 请求，直接向浏览器返回成功响应
		inboundConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	} else {
		// 如果是普通 HTTP 请求，将浏览器发来的数据直接发给目标网站
		if _, err := outboundConn.Write(a.initialData); err != nil {
			return
		}
	}

	// 3. 开始双向、无加密的数据转发
	a.relay(inboundConn, outboundConn)
}

// relay 是一个简单的、无加密的双向数据转发器
func (a *DirectAgent) relay(conn1, conn2 net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		io.Copy(conn1, conn2)
		conn1.Close()
		conn2.Close()
	}()

	go func() {
		defer wg.Done()
		io.Copy(conn2, conn1)
		conn1.Close()
		conn2.Close()
	}()

	wg.Wait()
}
