// --- START OF COMPLETE REPLACEMENT for worker_strategy.go ---
package socks5

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"liuproxy_go/internal/core/securecrypt"
	"liuproxy_go/internal/globalstate"
	"liuproxy_go/internal/protocol"
	"liuproxy_go/internal/shared"
	"liuproxy_go/internal/types"
	"net"
	"net/url"
	"strconv"
	"sync"
	"time"
)

// WorkerStrategy 实现了与Cloudflare Worker后端通信的策略。
// 它为每个客户端连接创建一个独立的WebSocket隧道，并实现非对称加密负载：
// - 上行 (Client -> Worker): 加密
// - 下行 (Worker -> Client): 明文 (由WSS的TLS保护)
type WorkerStrategy struct {
	config  *types.Config
	profile *BackendProfile
}

func NewWorkerStrategy(cfg *types.Config, profile *BackendProfile) (TunnelStrategy, error) {
	return &WorkerStrategy{
		config:  cfg,
		profile: profile,
	}, nil
}

// Initialize for WorkerStrategy is a no-op because it doesn't pre-connect.
func (s *WorkerStrategy) Initialize() error {
	return nil
}

// HandleConnection 是Worker策略的入口。它负责协议嗅探、连接建立和数据转发。
func (s *WorkerStrategy) HandleConnection(plainConn net.Conn, reader *bufio.Reader) {
	defer plainConn.Close()

	// 1. 协议嗅探，确定目标地址
	initialData, hostname, port, isSSL, err := GetTargetFromHTTPHeader(reader)
	if err != nil {
		// 如果不是HTTP(S)，则尝试SOCKS5握手
		var targetAddr string
		cmd, targetAddr, handshakeErr := NewAgent(s.config, s.config.CommonConf.BufferSize).(*Agent).handshakeWithClient(plainConn, reader)
		if handshakeErr != nil {
			return
		}
		if cmd == 3 { // UDP ASSOCIATE
			_, _ = plainConn.Write([]byte{0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0}) // Command not supported
			return
		}
		if cmd != 1 { // CONNECT
			return
		}
		hostname, _, _ = net.SplitHostPort(targetAddr)
		port, _ = strconv.Atoi(targetAddr[len(hostname)+1:])
		isSSL = true // SOCKS5 CONNECT 总是被视为SSL隧道
		initialData = nil
	}
	targetAddr := net.JoinHostPort(hostname, strconv.Itoa(port))

	// 2. 创建一个独立的隧道到Worker
	tunnelConn, cipher, err := s.createTunnel()
	if err != nil {
		return
	}
	defer tunnelConn.Close()

	// 3. 发送NewStream请求 (上行加密)
	packet := protocol.Packet{
		StreamID: 1, // 对于独立连接，StreamID总是1
		Flag:     protocol.FlagControlNewStreamTCP,
		Payload:  buildMetadataForWorker(1, targetAddr),
	}
	if err := protocol.WriteSecurePacket(tunnelConn, &packet, cipher); err != nil {
		return
	}

	// 4. 等待Worker的成功响应 (下行明文)
	if err := s.waitForSuccess(tunnelConn); err != nil {
		if initialData != nil { // 如果是HTTP请求，返回错误网关
			_, _ = plainConn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		}
		return
	}

	// 5. 向客户端发送成功响应
	if initialData != nil { // HTTP/HTTPS
		if isSSL {
			_, _ = plainConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
		}
	} else { // SOCKS5
		_, _ = plainConn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
	}

	globalstate.GlobalStatus.Set(fmt.Sprintf("Connected (Worker via %s)", s.profile.Address))

	// 6. 启动双向数据转发
	var wg sync.WaitGroup
	wg.Add(2)

	// 上行: plainConn -> Encrypt -> tunnelConn
	go func() {
		defer wg.Done()
		defer tunnelConn.Close()
		buf := make([]byte, s.config.CommonConf.BufferSize)
		// 如果是普通HTTP，先发送初始数据
		if len(initialData) > 0 && !isSSL {
			packet := protocol.Packet{StreamID: 1, Flag: protocol.FlagTCPData, Payload: initialData}
			if err := protocol.WriteSecurePacket(tunnelConn, &packet, cipher); err != nil {
				return
			}
		}
		// 循环读取并转发
		for {
			n, err := plainConn.Read(buf)
			if err != nil {
				// 发送关闭信号
				closePacket := protocol.Packet{StreamID: 1, Flag: protocol.FlagControlCloseStream}
				_ = protocol.WriteSecurePacket(tunnelConn, &closePacket, cipher)
				return
			}
			packet := protocol.Packet{StreamID: 1, Flag: protocol.FlagTCPData, Payload: buf[:n]}
			if err := protocol.WriteSecurePacket(tunnelConn, &packet, cipher); err != nil {
				return
			}
		}
	}()

	// 下行: tunnelConn -> No Decrypt -> plainConn
	go func() {
		defer wg.Done()
		defer plainConn.Close()
		for {
			// 使用新的非安全读取函数
			packet, err := protocol.ReadUnsecurePacket(tunnelConn)
			if err != nil {
				return
			}
			if packet.Flag == protocol.FlagTCPData {
				if _, err := plainConn.Write(packet.Payload); err != nil {
					return
				}
			} else if packet.Flag == protocol.FlagControlCloseStream {
				return
			}
		}
	}()

	wg.Wait()
}

// createTunnel 负责建立到Worker的WebSocket连接并创建加密器
func (s *WorkerStrategy) createTunnel() (net.Conn, *securecrypt.Cipher, error) {
	u := url.URL{
		Scheme: s.profile.Scheme,
		Host:   net.JoinHostPort(s.profile.Address, s.profile.Port),
		Path:   s.profile.Path,
	}

	var tunnelConn net.Conn
	var err error
	if s.profile.EdgeIP != "" {
		tunnelConn, err = shared.NewWebSocketConnAdapterClientWithEdgeIP(u.String(), s.profile.EdgeIP)
	} else {
		tunnelConn, err = shared.NewWebSocketConnAdapterClient(u.String())
	}
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to worker backend %s: %w", u.String(), err)
	}

	cipher, err := securecrypt.NewCipherWithAlgo(s.config.CommonConf.Crypt, securecrypt.AES_256_GCM)
	if err != nil {
		_ = tunnelConn.Close()
		return nil, nil, fmt.Errorf("failed to create AES-GCM cipher: %w", err)
	}

	return tunnelConn, cipher, nil
}

// waitForSuccess 等待Worker的成功响应 (明文)
func (s *WorkerStrategy) waitForSuccess(conn net.Conn) error {
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	defer conn.SetReadDeadline(time.Time{})

	// 使用新的非安全读取函数
	packet, err := protocol.ReadUnsecurePacket(conn)
	if err != nil {
		return err
	}

	if packet.Flag != protocol.FlagControlNewStreamTCPSuccess {
		return fmt.Errorf("unexpected flag from worker: got %d, want %d", packet.Flag, protocol.FlagControlNewStreamTCPSuccess)
	}

	return nil
}

func (s *WorkerStrategy) GetType() string {
	return "worker"
}

// buildMetadataForWorker (helper function, no change)
func buildMetadataForWorker(cmd byte, targetAddr string) []byte {
	host, portStr, _ := net.SplitHostPort(targetAddr)
	port, _ := strconv.Atoi(portStr)
	addrBytes := []byte(host)
	addrType := byte(0x03)
	if ip := net.ParseIP(host); ip != nil {
		if ipv4 := ip.To4(); ipv4 != nil {
			addrType = 0x01
			addrBytes = ipv4
		} else {
			addrType = 0x04
			addrBytes = ip.To16()
		}
	}
	var buf bytes.Buffer
	buf.WriteByte(cmd)
	buf.WriteByte(addrType)
	if addrType == 0x03 {
		buf.WriteByte(byte(len(addrBytes)))
	}
	buf.Write(addrBytes)
	_ = binary.Write(&buf, binary.BigEndian, uint16(port))
	return buf.Bytes()
}

func (s *WorkerStrategy) CloseTunnel() {
	// Nothing to do
}

// UpdateServers is a no-op for WorkerStrategy as it only uses one profile.
func (s *WorkerStrategy) UpdateServers(profiles []*BackendProfile) {
	// Nothing to do
}

func (s *WorkerStrategy) GetStatus() string {
	return "Connected (Worker Mode)"
}

func (s *WorkerStrategy) GetTunnelManager() *TunnelManager {
	return nil
}
