package goremote

import (
	"io"
	"liuproxy_go/internal/shared/logger"
	"liuproxy_go/internal/shared/protocol"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// SessionManager 管理所有来自本地客户端的会话 (Session)
type SessionManager struct {
	sessions     sync.Map
	streamPipes  sync.Map
	nextStreamID uint32
	agent        *Agent
}

func NewSessionManager(agent *Agent) *SessionManager {
	return &SessionManager{
		agent:        agent,
		nextStreamID: 0,
	}
}

// Session 代表一个来自本地客户端的会话。
type Session struct {
	streamID    uint16
	plainConn   net.Conn
	agent       *Agent
	closeOnce   sync.Once
	readyChan   chan bool
	doneChan    chan struct{}
	initialData []byte
	isSSL       bool
}

func NewSession(streamID uint16, plainConn net.Conn, agent *Agent, initialData []byte, isSSL bool) *Session {
	return &Session{
		streamID:    streamID,
		plainConn:   plainConn,
		agent:       agent,
		readyChan:   make(chan bool, 1),
		doneChan:    make(chan struct{}),
		initialData: initialData,
		isSSL:       isSSL,
	}
}

func (sm *SessionManager) NewTCPSession(plainConn net.Conn, targetAddr string, initialData []byte, isSSL bool) *Session {
	streamID := atomic.AddUint32(&sm.nextStreamID, 1)
	if streamID > 65530 {
		atomic.StoreUint32(&sm.nextStreamID, 1)
		streamID = 1
	}
	session := NewSession(uint16(streamID), plainConn, sm.agent, initialData, isSSL)
	sm.sessions.Store(uint16(streamID), session)
	go session.Start(targetAddr)
	return session
}

func (s *Session) Start(targetAddr string) {
	defer s.Close()
	metadata := s.agent.BuildMetadata(1, targetAddr)
	packet := protocol.Packet{StreamID: s.streamID, Flag: protocol.FlagControlNewStreamTCP, Payload: metadata}
	if err := s.agent.WritePacket(&packet); err != nil {
		return
	}

	// 立即响应客户端，告知连接已建立
	if s.initialData != nil {
		if s.isSSL {
			_, _ = s.plainConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
		}
	} else {
		_, _ = s.plainConn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
	}

	// 等待远程服务器确认
	select {
	case <-s.readyChan:
		// 远程已就绪，可以开始转发数据
	case <-time.After(10 * time.Second):
		return
	}

	tunnelStream := NewTunnelStream(s.streamID, s.agent)
	sm := s.agent.sessionManager
	sm.SetStreamPipe(s.streamID, tunnelStream)

	if len(s.initialData) > 0 && !s.isSSL {
		if _, err := tunnelStream.Write(s.initialData); err != nil {
			return
		}
	}
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		//defer tunnelStream.Close()
		io.Copy(tunnelStream, s.plainConn)
	}()
	go func() {
		defer wg.Done()
		defer s.plainConn.Close()
		io.Copy(s.plainConn, tunnelStream)
	}()
	wg.Wait()
}

func (s *Session) SignalConnectionSuccess() {
	select {
	case s.readyChan <- true:
	default:
	}
}
func (s *Session) Close() {
	s.closeOnce.Do(func() {
		s.plainConn.Close()
		s.agent.sessionManager.RemoveStreamPipe(s.streamID)
		_ = s.agent.WritePacket(&protocol.Packet{StreamID: s.streamID, Flag: protocol.FlagControlCloseStream})
		close(s.doneChan)
	})
}
func (s *Session) Wait() { <-s.doneChan }
func (sm *SessionManager) GetSession(streamID uint16) *Session {
	if s, ok := sm.sessions.Load(streamID); ok {
		return s.(*Session)
	}
	return nil
}

func (sm *SessionManager) RemoveSession(streamID uint16) {
	if s, loaded := sm.sessions.LoadAndDelete(streamID); loaded {
		s.(*Session).Close()
	}
}

// CloseAllSessions 强制关闭所有当前活动的会话。
func (sm *SessionManager) CloseAllSessions() {
	sm.sessions.Range(func(key, value interface{}) bool {
		if session, ok := value.(*Session); ok {
			session.Close()
		}
		return true
	})
}

const dataChannelSize = 512

type TunnelStream struct {
	streamID  uint16
	agent     *Agent
	dataChan  chan []byte
	closeOnce sync.Once
}

func NewTunnelStream(streamID uint16, agent *Agent) *TunnelStream {
	return &TunnelStream{streamID: streamID, agent: agent, dataChan: make(chan []byte, dataChannelSize)}
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
	dataCopy := make([]byte, len(p))
	copy(dataCopy, p)
	packet := protocol.Packet{StreamID: ts.streamID, Flag: protocol.FlagTCPData, Payload: dataCopy}
	if err := ts.agent.WritePacket(&packet); err != nil {
		return 0, err
	}
	return len(p), nil
}
func (ts *TunnelStream) PushDownstreamData(data []byte) {
	select {
	case ts.dataChan <- data:
	default:
		logger.Debug().Uint16("streamID", ts.streamID).Msg("[TunnelStream] Downstream channel full")
	}
}
func (ts *TunnelStream) Close() error { ts.closeOnce.Do(func() { close(ts.dataChan) }); return nil }
func (sm *SessionManager) SetStreamPipe(streamID uint16, pipe *TunnelStream) {
	sm.streamPipes.Store(streamID, pipe)
}
func (sm *SessionManager) GetStreamPipe(streamID uint16) *TunnelStream {
	if pipe, ok := sm.streamPipes.Load(streamID); ok {
		return pipe.(*TunnelStream)
	}
	return nil
}
func (sm *SessionManager) RemoveStreamPipe(streamID uint16) {
	if pipe, loaded := sm.streamPipes.LoadAndDelete(streamID); loaded {
		pipe.(*TunnelStream).Close()
	}
}
