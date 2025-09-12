// --- START OF COMPLETE REPLACEMENT for remote_session.go ---
package socks5

import (
	"net"
	"sync"
	"time"

	"liuproxy_go/internal/protocol"
)

// RemoteSession 代表一个到最终目标服务器的连接。
// 它从RemoteTunnel接收明文指令和数据。
type RemoteSession struct {
	streamID   uint16
	tunnel     *RemoteTunnel
	targetConn net.Conn
	closeOnce  sync.Once
}

func NewRemoteSession(streamID uint16, tunnel *RemoteTunnel) *RemoteSession {
	return &RemoteSession{
		streamID: streamID,
		tunnel:   tunnel,
	}
}

// Connect 方法解析明文元数据并连接到最终目标。
func (s *RemoteSession) Connect(payload []byte) (string, error) {
	// payload 已经是明文，直接解析
	_, targetAddr, err := s.tunnel.agent.parseMetadata(payload)
	if err != nil {
		return "", err
	}
	conn, err := net.DialTimeout("tcp", targetAddr, 10*time.Second)
	if err != nil {
		return targetAddr, err
	}
	s.targetConn = conn
	return targetAddr, nil
}

// StartDownstreamLoop 启动一个循环，从目标服务器读取明文数据，
// 并将其封装成Packet交由隧道发送（隧道会负责加密）。
func (s *RemoteSession) StartDownstreamLoop() {
	defer s.Close()
	defer func() {
		if s.tunnel != nil {
			_ = s.tunnel.WritePacket(&protocol.Packet{StreamID: s.streamID, Flag: protocol.FlagControlCloseStream})
			s.tunnel.sessionManager.RemoveSession(s.streamID)
		}
	}()

	if s.targetConn == nil {
		return
	}
	if s.tunnel == nil || s.tunnel.agent == nil {
		return
	}

	buf := make([]byte, s.tunnel.agent.bufferSize)
	for {
		n, err := s.targetConn.Read(buf)
		if err != nil {
			return
		}

		// 将读取到的明文数据封装成Packet
		payloadCopy := make([]byte, n)
		copy(payloadCopy, buf[:n])
		packet := protocol.Packet{
			StreamID: s.streamID,
			Flag:     protocol.FlagTCPData,
			Payload:  payloadCopy, // Payload是明文
		}
		// 将Packet交由隧道发送（隧道会负责加密）
		if err := s.tunnel.WritePacket(&packet); err != nil {
			return
		}
	}
}

// WriteToTarget 由RemoteTunnel调用，将解密后的明文数据写入到目标服务器。
func (s *RemoteSession) WriteToTarget(payload []byte) {
	if s.targetConn == nil {
		return
	}
	_, _ = s.targetConn.Write(payload)
}

func (s *RemoteSession) Close() {
	s.closeOnce.Do(func() {
		if s.targetConn != nil {
			_ = s.targetConn.Close()
		}
	})
}
