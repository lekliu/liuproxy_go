// --- START OF COMPLETE REPLACEMENT for local_session.go ---
package socks5

import (
	"io"
	"log"
	"net"
	"sync"
	"time"

	"liuproxy_go/internal/protocol"
)

// LocalSession 代表一个来自本地客户端的会话。
// 它负责处理与本地应用（如浏览器）的明文通信，
// 并通过TunnelManager将数据封装成Packet进行转发。
type LocalSession struct {
	streamID      uint16
	plainConn     net.Conn
	tunnelManager *TunnelManager
	closeOnce     sync.Once
	readyChan     chan bool
	doneChan      chan struct{}
	initialData   []byte
	isSSL         bool
}

// NewLocalSession 创建一个新的本地会话。
// 注意：所有参数都是关于明文通信和隧道管理的，不涉及加密。
func NewLocalSession(streamID uint16, plainConn net.Conn, tm *TunnelManager, initialData []byte, isSSL bool) *LocalSession {
	return &LocalSession{
		streamID:      streamID,
		plainConn:     plainConn,
		tunnelManager: tm,
		readyChan:     make(chan bool, 1),
		doneChan:      make(chan struct{}),
		initialData:   initialData,
		isSSL:         isSSL,
	}
}

// Start 启动会话处理流程。
func (s *LocalSession) Start(targetAddr string) {
	defer s.Close()

	// 1. 构建包含明文元数据的Packet
	metadata := s.tunnelManager.agent.buildMetadata(1, targetAddr)
	packet := protocol.Packet{
		StreamID: s.streamID,
		Flag:     protocol.FlagControlNewStreamTCP,
		Payload:  metadata, // Payload是明文
	}

	// 2. 将Packet交由TunnelManager处理（TunnelManager会负责加密）
	if err := s.tunnelManager.WritePacket(&packet); err != nil {
		return
	}

	// 3. 等待远端的成功响应（由TunnelManager的ReadLoop接收并触发）
	select {
	case <-s.readyChan:
		// 向本地应用（浏览器/SOCKS客户端）发送成功响应
		if s.initialData != nil { // HTTP/HTTPS 代理
			if s.isSSL {
				_, _ = s.plainConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
			}
		} else { // SOCKS5 代理
			_, _ = s.plainConn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		}

	case <-time.After(10 * time.Second):
		if s.initialData != nil { // HTTP/HTTPS 代理
			_, _ = s.plainConn.Write([]byte("HTTP/1.1 504 Gateway Timeout\r\n\r\n"))
		}
		return
	}

	// 4. 创建一个虚拟的流管道，用于在会话和隧道之间传递明文数据
	tunnelStream := NewTunnelStream(s.streamID, s.tunnelManager)
	s.tunnelManager.SessionManager.SetStreamPipe(s.streamID, tunnelStream)

	// 5. 如果是普通HTTP代理，将初始数据写入隧道
	if len(s.initialData) > 0 && !s.isSSL {
		if _, err := tunnelStream.Write(s.initialData); err != nil {
			return
		}
	}

	// 6. 启动双向数据拷贝循环
	var wg sync.WaitGroup
	wg.Add(2)

	// 上行: 从本地应用读取明文 -> 写入虚拟管道 -> TunnelManager加密并发送
	go func() {
		defer wg.Done()
		defer tunnelStream.Close()
		io.Copy(tunnelStream, s.plainConn)
	}()

	// 下行: TunnelManager解密并Push数据 -> 从虚拟管道读取明文 -> 写入本地应用
	go func() {
		defer wg.Done()
		defer s.plainConn.Close()
		io.Copy(s.plainConn, tunnelStream)

	}()

	wg.Wait()
}
func isUseOfClosedNetworkError(err error) bool {
	if opErr, ok := err.(*net.OpError); ok {
		return opErr.Err.Error() == "use of closed network connection"
	}
	return false
}
func (s *LocalSession) SignalConnectionSuccess() {
	select {
	case s.readyChan <- true:
	default:
	}
}
func (s *LocalSession) Close() {
	s.closeOnce.Do(func() {
		s.plainConn.Close()
		s.tunnelManager.SessionManager.RemoveStreamPipe(s.streamID)
		_ = s.tunnelManager.WritePacket(&protocol.Packet{StreamID: s.streamID, Flag: protocol.FlagControlCloseStream, Payload: nil})
		close(s.doneChan)
	})
}
func (s *LocalSession) Wait() {
	<-s.doneChan
}

const dataChannelSize = 512

// TunnelStream 是一个内存中的管道，用于在会话的IO循环和隧道的IO循环之间解耦和缓冲数据。
// 它处理的始终是明文数据。
type TunnelStream struct {
	streamID      uint16
	tunnelManager *TunnelManager
	dataChan      chan []byte
	closeOnce     sync.Once
}

func NewTunnelStream(streamID uint16, tm *TunnelManager) *TunnelStream {
	return &TunnelStream{
		streamID:      streamID,
		tunnelManager: tm,
		dataChan:      make(chan []byte, dataChannelSize),
	}
}
func (ts *TunnelStream) Read(p []byte) (n int, err error) {
	data, ok := <-ts.dataChan
	if !ok {
		return 0, io.EOF
	}
	n = copy(p, data)
	return n, nil
}
func (ts *TunnelStream) Write(p []byte) (n int, err error) {
	// 复制数据以确保生命周期安全
	dataCopy := make([]byte, len(p))
	copy(dataCopy, p)

	packet := protocol.Packet{
		StreamID: ts.streamID,
		Flag:     protocol.FlagTCPData,
		Payload:  dataCopy, // Payload是明文TCP数据
	}
	// 将Packet交由TunnelManager处理（加密并发送）
	if err := ts.tunnelManager.WritePacket(&packet); err != nil {
		return 0, err
	}
	return len(p), nil
}

// PushDownstreamData 由TunnelManager的ReadLoop调用，将解密后的明文数据推入管道。
func (ts *TunnelStream) PushDownstreamData(data []byte) {
	select {
	case ts.dataChan <- data:
	default:
		log.Printf("[TunnelStream] Stream %d: Downstream channel full, dropping packet of size %d", ts.streamID, len(data))
	}
}
func (ts *TunnelStream) Close() error {
	ts.closeOnce.Do(func() {
		close(ts.dataChan)
	})
	return nil
}
