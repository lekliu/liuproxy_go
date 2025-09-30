// --- START OF COMPLETE REPLACEMENT for liuproxy_go/internal/goremote/udp_manager.go ---
package goremote

import (
	"fmt"
	"net"
	"strconv"
	"sync"
)

type UDPManager struct {
	agent            *Agent
	bufferSize       int
	sessionMux       sync.Mutex
	singletonSession *UDPSession
}

func NewUDPManager(agent *Agent, bufferSize int) *UDPManager {
	return &UDPManager{
		agent:      agent,
		bufferSize: bufferSize,
	}
}

func (m *UDPManager) GetOrCreateSingletonSession() (*UDPSession, error) {
	m.sessionMux.Lock()
	defer m.sessionMux.Unlock()

	if m.singletonSession != nil && m.singletonSession.IsRunning() {
		return m.singletonSession, nil
	}
	listener, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("udp manager failed to listen: %w", err)
	}
	session := &UDPSession{
		agent:         m.agent,
		localListener: listener,
		stopChan:      make(chan struct{}),
	}
	m.singletonSession = session
	go session.runUpstreamLoop()
	return session, nil
}

func (m *UDPManager) HandleDownstreamPacket(payload []byte) {
	srcHost, srcPort := parseSocks5UDPTarget(payload)
	srcAddrStr := net.JoinHostPort(srcHost, strconv.Itoa(srcPort))

	session := m.singletonSession
	if session == nil || !session.IsRunning() {
		return
	}
	clientVal, found := session.reverseMap.Load(srcAddrStr)
	if !found {
		return
	}
	clientUDPAddr, err := net.ResolveUDPAddr("udp", clientVal.(string))
	if err != nil {
		return
	}
	session.localListener.WriteTo(payload, clientUDPAddr)
}

// --- END OF COMPLETE REPLACEMENT ---
