// --- START OF COMPLETE REPLACEMENT for local_session_manager.go ---
package socks5

import (
	"net"
	"sync"
	"sync/atomic"
)

// LocalSessionManager 管理所有来自本地客户端的会-话 (LocalSession)
type LocalSessionManager struct {
	sessions      sync.Map
	streamPipes   sync.Map
	nextStreamID  uint32
	tunnelManager *TunnelManager
}

// NewLocalSessionManager 创建一个新的会话管理器
func NewLocalSessionManager() *LocalSessionManager {
	return &LocalSessionManager{nextStreamID: 0}
}

// NewTCPSession 为一个新的客户端连接创建一个新的 TCP 会话 (SOCKS5 和 HTTP 代理共用)
func (sm *LocalSessionManager) NewTCPSession(plainConn net.Conn, targetAddr string, initialData []byte, isSSL bool) *LocalSession {
	// 使用原子操作来安全地获取下一个流 ID
	streamID := atomic.AddUint32(&sm.nextStreamID, 1)
	// 防止 streamID 无限增长，在一个合理的范围循环
	if streamID > 65530 {
		atomic.StoreUint32(&sm.nextStreamID, 1)
		streamID = 1
	}

	session := NewLocalSession(uint16(streamID), plainConn, sm.tunnelManager, initialData, isSSL)
	sm.sessions.Store(uint16(streamID), session)

	// 在一个新的 goroutine 中启动会话，避免阻塞
	go session.Start(targetAddr)
	return session
}

// GetSession 根据流 ID 获取一个会话
func (sm *LocalSessionManager) GetSession(streamID uint16) *LocalSession {
	if s, ok := sm.sessions.Load(streamID); ok {
		return s.(*LocalSession)
	}
	return nil
}

// RemoveSession 移除并关闭一个会话
func (sm *LocalSessionManager) RemoveSession(streamID uint16) {
	if s, loaded := sm.sessions.LoadAndDelete(streamID); loaded {
		s.(*LocalSession).Close()
	}
}

// CloseAll 在隧道断开时，关闭所有活跃的会话
func (sm *LocalSessionManager) CloseAll() {
	sm.sessions.Range(func(key, value interface{}) bool {
		sm.RemoveSession(key.(uint16))
		return true
	})
}

// SetStreamPipe 存储一个用于数据转发的 TunnelStream
func (sm *LocalSessionManager) SetStreamPipe(streamID uint16, pipe *TunnelStream) {
	sm.streamPipes.Store(streamID, pipe)
}

// GetStreamPipe 根据流 ID 获取一个 TunnelStream
func (sm *LocalSessionManager) GetStreamPipe(streamID uint16) *TunnelStream {
	if pipe, ok := sm.streamPipes.Load(streamID); ok {
		return pipe.(*TunnelStream)
	}
	return nil
}

// RemoveStreamPipe 移除并关闭一个 TunnelStream
func (sm *LocalSessionManager) RemoveStreamPipe(streamID uint16) {
	if pipe, loaded := sm.streamPipes.LoadAndDelete(streamID); loaded {
		pipe.(*TunnelStream).Close()
	}
}

// --- END OF COMPLETE REPLACEMENT ---
