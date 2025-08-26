package agent

import (
	"encoding/binary"
	"fmt"
	"io"
	"liuproxy_go/internal/core/crypt"
	"net"
	"strconv"
)

const SocksVersion = 5

// handle 是 SOCKS5 代理的核心处理逻辑
func (a *Socks5Agent) handle(inboundConn net.Conn) {
	var outboundConn net.Conn
	var err error

	if a.config.IsServerSide {
		outboundConn, err = a.remoteHandshake(inboundConn)
	} else {
		outboundConn, err = a.localHandshake(inboundConn)
	}

	if err != nil {
		inboundConn.Close()
		return
	}

	// 检查 localHandshake/remoteHandshake 的返回值。
	// 在UDP流程中，它们会返回 (nil, nil) 来表示 "处理已转交，无需继续"。
	if outboundConn == nil {
		return
	}

	// 如果代码执行到这里，说明这是一个CONNECT流程，我们需要建立TCP中继。
	// 现在可以安全地设置defer来关闭两个连接了。
	defer inboundConn.Close()
	defer outboundConn.Close()

	relayCfg := RelayConfig{
		IsServerSide: a.config.IsServerSide,
		EncryptKey:   a.config.EncryptKey,
		BufferSize:   a.bufferSize,
	}
	TCPRelay(inboundConn, outboundConn, relayCfg)
}

// localHandshake 精确复现了原 local/socks5.go 的逻辑，并修正了TCP流处理问题
func (a *Socks5Agent) localHandshake(inboundConn net.Conn) (net.Conn, error) {
	// 连接到 remote server
	outboundConn, err := net.Dial("tcp", net.JoinHostPort(a.config.RemoteHost, strconv.Itoa(a.config.RemotePort)))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to remote SOCKS5 server: %w", err)
	}

	buf := make([]byte, 262)

	// 步骤 1: 处理认证阶段 (Authentication)
	// 从客户端读取认证信息
	n, err := inboundConn.Read(buf)
	if err != nil {
		outboundConn.Close()
		return nil, fmt.Errorf("failed to read auth request from client: %w", err)
	}
	// 加密并转发给 remote
	if _, err := outboundConn.Write(crypt.Encrypt(buf[:n], a.config.EncryptKey)); err != nil {
		outboundConn.Close()
		return nil, fmt.Errorf("failed to write auth request to remote: %w", err)
	}

	// 从 remote 读取认证响应
	n, err = outboundConn.Read(buf)
	if err != nil {
		outboundConn.Close()
		return nil, fmt.Errorf("failed to read auth response from remote: %w", err)
	}
	// 解密并转发给客户端

	decryptedAuthReply := crypt.Decrypt(buf[:n], a.config.EncryptKey)
	if _, err := inboundConn.Write(decryptedAuthReply); err != nil {
		outboundConn.Close()
		return nil, fmt.Errorf("failed to write auth response to client: %w", err)
	}

	// 步骤 2: 处理命令阶段 (Command)
	// 现在，认证已完成，单独读取命令请求
	n, err = inboundConn.Read(buf)

	if err != nil {
		outboundConn.Close()
		return nil, fmt.Errorf("failed to read command request from client: %w", err)
	}

	clientRequest := buf[:n]
	cmd := clientRequest[1]
	// 加密并转发给 remote
	if _, err := outboundConn.Write(crypt.Encrypt(clientRequest, a.config.EncryptKey)); err != nil {
		outboundConn.Close()
		return nil, fmt.Errorf("failed to write command request to remote: %w", err)
	}

	// 步骤 3: 根据命令码，决定后续行为
	if cmd == 1 { // CONNECT
		// 对于CONNECT，local端需要继续转发remote的响应，然后进入TCPRelay
		n, err = outboundConn.Read(buf)
		if err != nil {
			outboundConn.Close()
			return nil, err
		}
		inboundConn.Write(crypt.Decrypt(buf[:n], a.config.EncryptKey))
		return outboundConn, nil // 交给上层TCPRelay

	} else if cmd == 3 { // UDP ASSOCIATE
		// 对于UDP，local端需要接收remote的响应，但不是为了转发，而是为了确认remote已准备好
		// 并获取remote准备好的UDP地址，以供后续转发使用。
		n, err = outboundConn.Read(buf)
		if err != nil {
			outboundConn.Close()
			return nil, fmt.Errorf("failed to read UDP associate confirmation from remote: %w", err)
		}

		// 解密来自 remote 的响应
		remoteResponse := crypt.Decrypt(buf[:n], a.config.EncryptKey)
		if len(remoteResponse) < 10 {
			outboundConn.Close()
			return nil, fmt.Errorf("invalid UDP associate response from remote, length < 10")
		}

		// 从remote的响应中解析出它监听的UDP端口
		remoteUdpPort := int(binary.BigEndian.Uint16(remoteResponse[8:10]))
		remoteUdpAddrStr := fmt.Sprintf("%s:%d", a.config.RemoteHost, remoteUdpPort)
		remoteUdpAddr, err := net.ResolveUDPAddr("udp", remoteUdpAddrStr)
		if err != nil {
			outboundConn.Close()
			return nil, fmt.Errorf("could not resolve remote UDP address: %w", err)
		}

		// 现在，将客户端的TCP连接、到Remote的TCP连接以及Remote的UDP地址
		// 全部交给UDP处理器。由它来完成最后的响应和数据转发。
		// 【重要】我们不再向 inboundConn 写入任何东西！
		go a.handleUdpAssociateLocal(inboundConn, outboundConn, remoteUdpAddr)

		// UDP流程已经启动，主TCPRelay不需要运行
		return nil, nil
	}

	outboundConn.Close()
	return nil, fmt.Errorf("unsupported command: %d", cmd)
}

// remoteHandshake 精确复现了原 remote/socks5.go 的逻辑
func (a *Socks5Agent) remoteHandshake(inboundConn net.Conn) (net.Conn, error) {
	// 1. 认证阶段
	authEncrypted := make([]byte, 3)
	if _, err := io.ReadFull(inboundConn, authEncrypted); err != nil {
		return nil, err
	}
	auth := crypt.Decrypt(authEncrypted, a.config.EncryptKey)
	if len(auth) < 3 || auth[0] != SocksVersion || auth[1] != 1 || auth[2] != 0 {
		return nil, fmt.Errorf("SOCKS5 auth failed")
	}
	// 回应认证成功
	responseEncrypted := crypt.Encrypt([]byte{SocksVersion, 0}, a.config.EncryptKey)
	if _, err := inboundConn.Write(responseEncrypted); err != nil {
		return nil, err
	}

	// 2. 连接请求阶段
	// 一次性读取请求，避免TCP粘包问题
	bufEncrypted := make([]byte, 262)
	n, err := inboundConn.Read(bufEncrypted)
	if err != nil {
		return nil, err
	}
	req := crypt.Decrypt(bufEncrypted[:n], a.config.EncryptKey)
	if len(req) < 7 { // 最小请求长度
		return nil, fmt.Errorf("request packet too short")
	}

	cmd := req[1]

	if cmd == 1 { // CONNECT
		var address string
		var port int
		addrType := req[3]
		if addrType == 1 { // IPv4
			address = net.IP(req[4:8]).String()
			port = int(binary.BigEndian.Uint16(req[8:10]))
		} else if addrType == 3 { // Domain
			domainLen := int(req[4])
			address = string(req[5 : 5+domainLen])
			port = int(binary.BigEndian.Uint16(req[5+domainLen : 5+domainLen+2]))
		} else {
			return nil, fmt.Errorf("unsupported SOCKS5 address type: %d", addrType)
		}

		outboundConn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", address, port))
		if err != nil {
			inboundConn.Write(crypt.Encrypt([]byte{SocksVersion, 1, 0, 1, 0, 0, 0, 0, 0, 0}, a.config.EncryptKey))
			return nil, err
		}

		bindAddress := outboundConn.LocalAddr().(*net.TCPAddr)
		reply := []byte{SocksVersion, 0, 0, 1}
		reply = append(reply, bindAddress.IP.To4()...)
		portBytesReply := make([]byte, 2)
		binary.BigEndian.PutUint16(portBytesReply, uint16(bindAddress.Port))
		reply = append(reply, portBytesReply...)
		inboundConn.Write(crypt.Encrypt(reply, a.config.EncryptKey))
		return outboundConn, nil

	} else if cmd == 3 { // UDP ASSOCIATE
		a.handleUdpAssociateRemote(inboundConn)
		return nil, nil // UDP 流程自己管理连接
	}

	return nil, fmt.Errorf("unsupported SOCKS5 command: %d", cmd)
}
