// --- START OF COMPLETE REPLACEMENT for remote_session_manager.go ---
package socks5

import (
	"sync"

	"liuproxy_go/internal/protocol"
)

// RemoteSessionManager 管理所有来自隧道的远程会话 (RemoteSession)。
// 它从RemoteTunnel接收已经解密后的明文指令。
type RemoteSessionManager struct {
	sessions sync.Map
}

func NewRemoteSessionManager() *RemoteSessionManager {
	return &RemoteSessionManager{}
}

// NewTCPSession 为一个新的隧道流创建一个新的远程TCP会话。
func (sm *RemoteSessionManager) NewTCPSession(streamID uint16, payload []byte, tunnel *RemoteTunnel) {
	session := NewRemoteSession(streamID, tunnel)

	// payload 已经是解密后的明文元数据，直接交给session处理
	_, err := session.Connect(payload)
	if err != nil {
		// 连接目标失败，通知Local端关闭此流
		_ = tunnel.WritePacket(&protocol.Packet{StreamID: streamID, Flag: protocol.FlagControlCloseStream})
		return
	}

	sm.sessions.Store(streamID, session)

	// 连接目标成功，向Local端发送成功响应
	err = tunnel.WritePacket(&protocol.Packet{StreamID: streamID, Flag: protocol.FlagControlNewStreamTCPSuccess})
	if err != nil {
		sm.RemoveSession(streamID)
		return
	}

	// 启动下行数据转发循环
	go session.StartDownstreamLoop()
}

func (sm *RemoteSessionManager) GetSession(streamID uint16) *RemoteSession {
	if s, ok := sm.sessions.Load(streamID); ok {
		return s.(*RemoteSession)
	}
	return nil
}

func (sm *RemoteSessionManager) RemoveSession(streamID uint16) {
	if s, loaded := sm.sessions.LoadAndDelete(streamID); loaded {
		s.(*RemoteSession).Close()
	}
}

func (sm *RemoteSessionManager) CloseAll() {
	sm.sessions.Range(func(key, value interface{}) bool {
		sm.RemoveSession(key.(uint16))
		return true
	})
}

// --- END OF COMPLETE REPLACEMENT ---
