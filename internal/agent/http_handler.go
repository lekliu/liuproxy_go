// 修改点 1: 导入 "fmt" 包 **********
// Modification Point 1: Import the "fmt" package **********
// 原始行号: 3
package agent

import (
	"io"
	"net"
	"strconv"

	"liuproxy_go/internal/core/crypt"
	"liuproxy_go/pkg/geoip"
)

// handle 是 HTTPAgent 的核心逻辑入口
func (a *HTTPAgent) handle(inboundConn net.Conn) {
	defer inboundConn.Close()

	if a.config.IsServerSide {
		a.handleRemote(inboundConn)
	} else {
		a.handleLocal(inboundConn)
	}
}

// handleLocal 复现了原 local/proxy.go 的完整逻辑
func (a *HTTPAgent) handleLocal(inboundConn net.Conn) {
	// --- Handshake 阶段 ---
	// 1. 解析目标地址
	initialData, hostname, port, isSSL := getTargetFromHTTPHeader(inboundConn, a.bufferSize, false, 0)
	if hostname == "" {
		return
	}

	// 2. GeoIP 检查
	switch geoip.GeoIP(hostname) {
	case 0: // 断开
		return
	case 1: // 直连
		directAgent := NewDirectAgent(hostname, port, isSSL, initialData, a.bufferSize)
		directAgent.HandleConnection(inboundConn)
		return
	}

	// 3. 建立到 Remote 的出站连接
	outboundConn, err := net.Dial("tcp", net.JoinHostPort(a.config.RemoteHost, strconv.Itoa(a.config.RemotePort)))
	if err != nil {
		return
	}
	defer outboundConn.Close()

	// 4. 发送加密的初始请求 (带头部)
	encryptedRequest := crypt.EncryptWithHeader(initialData, a.config.EncryptKey)
	if _, err := outboundConn.Write(encryptedRequest); err != nil {
		return
	}

	// 5. 接收并解密初始响应
	buf := make([]byte, a.bufferSize)
	n, err := outboundConn.Read(buf)
	if err != nil {
		if err != io.EOF {
			return
		}
	}
	decryptedResponse := crypt.Decrypt(buf[:n], a.config.EncryptKey)
	if _, err := inboundConn.Write(decryptedResponse); err != nil {
		return
	}
	// --- Handshake 结束 ---

	// --- Relay 阶段 ---
	relayCfg := RelayConfig{
		IsServerSide: a.config.IsServerSide,
		EncryptKey:   a.config.EncryptKey,
		BufferSize:   a.bufferSize,
	}
	TCPRelay(inboundConn, outboundConn, relayCfg)
}

// handleRemote 复现了原 remote/host.go 的完整逻辑
func (a *HTTPAgent) handleRemote(inboundConn net.Conn) {
	// --- Handshake 阶段 ---
	// 1. 解析来自 Local 的加密请求
	initialDataEncrypted, hostname, port, isSSL := getTargetFromHTTPHeader(inboundConn, a.bufferSize, true, a.config.EncryptKey)
	if hostname == "" {
		return
	}

	targetAddr := net.JoinHostPort(hostname, strconv.Itoa(port))

	outboundConn, err := net.Dial("tcp", targetAddr)
	if err != nil {
		return
	}
	// 修改点 6: 添加诊断日志 **********
	defer outboundConn.Close()

	// 3. 获取并加密初始响应
	var initialResponse []byte
	if isSSL {
		initialResponse = []byte("HTTP/1.1 200 Connection Established\r\n\r\n")
	} else {
		decryptedInitialData := crypt.DecryptWithHeader(initialDataEncrypted, a.config.EncryptKey)
		if _, err := outboundConn.Write(decryptedInitialData); err != nil {
			return
		}
		// 然后读取目标的响应
		buf := make([]byte, a.bufferSize)
		n, err := outboundConn.Read(buf)
		if err != nil {
			if err != io.EOF {
				return
			}
		}
		// 修改点 12: 添加诊断日志 **********
		initialResponse = buf[:n]
	}

	encryptedResponse := crypt.Encrypt(initialResponse, a.config.EncryptKey)
	if _, err := inboundConn.Write(encryptedResponse); err != nil {
		return
	}

	// --- Relay 阶段 ---
	relayCfg := RelayConfig{
		IsServerSide: a.config.IsServerSide,
		EncryptKey:   a.config.EncryptKey,
		BufferSize:   a.bufferSize,
	}
	TCPRelay(inboundConn, outboundConn, relayCfg)
}
