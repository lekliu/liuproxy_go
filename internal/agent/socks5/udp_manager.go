// --- START OF COMPLETE REPLACEMENT for udp_manager.go ---
package socks5

import (
	"fmt"
	"log"
	"net"
	"strconv"
	"sync"
)

// UDPManager 是 Local 端全局唯一的 UDP 会话管理器。
// 它处理SOCKS5 UDP Associate命令，并管理NAT映射。
// 它接收和发送的都是明文SOCKS5 UDP数据包。
type UDPManager struct {
	tunnelManager    *TunnelManager
	bufferSize       int
	sessionMux       sync.Mutex
	singletonSession *UDPSession
}

// NewUDPManager 创建一个新的 UDPManager 实例。
func NewUDPManager(tm *TunnelManager, bufferSize int) *UDPManager {
	return &UDPManager{
		tunnelManager: tm,
		bufferSize:    bufferSize,
	}
}

// GetOrCreateSingletonSession 获取或创建一个全局唯一的 UDP 会话实例。
func (m *UDPManager) GetOrCreateSingletonSession() (*UDPSession, error) {
	m.sessionMux.Lock()
	defer m.sessionMux.Unlock()

	// 如果已存在一个正在运行的会话，直接返回
	if m.singletonSession != nil && m.singletonSession.IsRunning() {
		return m.singletonSession, nil
	}

	// 创建一个新的 UDP 监听器
	listener, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("udp manager failed to listen: %w", err)
	}

	// 创建新的会话实例
	session := &UDPSession{
		manager:       m,
		localListener: listener,
		stopChan:      make(chan struct{}),
	}

	m.singletonSession = session

	// 在后台启动会话的上行处理循环
	go session.runUpstreamLoop()

	return session, nil
}

// HandleDownstreamPacket 是从隧道接收下行UDP数据的入口。
// payload 参数是已经由TunnelManager解密后的明文SOCKS5 UDP数据包。
func (m *UDPManager) HandleDownstreamPacket(payload []byte) {
	// payload 已经是明文，直接解析
	srcHost, srcPort := parseSocks5UDPTarget(payload)
	srcAddrStr := net.JoinHostPort(srcHost, strconv.Itoa(srcPort))

	session := m.singletonSession
	if session == nil || !session.IsRunning() {
		return
	}
	sessionID := session.localListener.LocalAddr().String()

	clientVal, found := session.reverseMap.Load(srcAddrStr)
	if !found {
		log.Printf("[UDPManager][%s] Downstream: NAT lookup FAILED. No client found in map for key '%s'. Packet dropped.", sessionID, srcAddrStr)
		// 调试：打印出当前NAT表的所有内容
		log.Printf("[UDPManager][%s] -------- NAT MAP DUMP --------", sessionID)
		session.reverseMap.Range(func(key, value interface{}) bool {
			log.Printf("[UDPManager][%s]   - MAP ENTRY: '%s' -> '%s'", sessionID, key, value)
			return true
		})
		log.Printf("[UDPManager][%s] ------------------------------", sessionID)
		return
	}

	clientAddrStr := clientVal.(string)
	clientUDPAddr, err := net.ResolveUDPAddr("udp", clientAddrStr)
	if err != nil {
		return
	}

	session.localListener.WriteTo(payload, clientUDPAddr)
}
