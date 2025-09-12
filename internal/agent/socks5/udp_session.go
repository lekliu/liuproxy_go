// --- START OF NEW FILE internal/agent/socks5/udp_session.go ---
package socks5

import (
	"encoding/binary"
	"liuproxy_go/internal/protocol"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// UDPSession 代表一个本地UDP会话实例
type UDPSession struct {
	manager       *UDPManager
	localListener net.PacketConn
	forwardMap    sync.Map // 正向NAT: clientAddr -> lastSeen
	reverseMap    sync.Map // 反向NAT: targetAddr -> clientAddr
	closeOnce     sync.Once
	stopChan      chan struct{}
	running       atomic.Bool
}

// GetListenerAddr 返回此会话正在监听的本地UDP地址
func (s *UDPSession) GetListenerAddr() net.Addr {
	return s.localListener.LocalAddr()
}

// runUpstreamLoop 是会话的核心，负责从客户端读取UDP包并转发到隧道
func (s *UDPSession) runUpstreamLoop() {
	s.running.Store(true)
	defer s.running.Store(false)
	defer s.Close()

	//sessionID := s.localListener.LocalAddr().String()
	buf := make([]byte, s.manager.bufferSize)

	for {
		// 设置读取超时，用于定期检查会话是否应关闭
		_ = s.localListener.SetReadDeadline(time.Now().Add(udpTimeout))
		n, clientAddr, err := s.localListener.ReadFrom(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				select {
				case <-s.stopChan: // 如果是主动关闭，则退出
					return
				default: // 如果是自然超时，则清理NAT表并继续
					s.cleanupNatMap()
					continue
				}
			}
			// 其他读取错误，退出循环
			return
		}

		// 解析SOCKS5 UDP包以获取最终目标地址
		targetHost, targetPort := parseSocks5UDPTarget(buf[:n])
		targetAddrStr := net.JoinHostPort(targetHost, strconv.Itoa(targetPort))
		clientAddrStr := clientAddr.String()

		// 更新NAT映射表
		s.forwardMap.Store(clientAddrStr, time.Now())
		s.reverseMap.Store(targetAddrStr, clientAddrStr)

		payloadCopy := make([]byte, n)
		copy(payloadCopy, buf[:n])

		// 封装成隧道协议包
		packet := protocol.Packet{
			StreamID: 0xFFFF, // UDP使用固定的StreamID
			Flag:     protocol.FlagUDPData,
			Payload:  payloadCopy,
		}

		// 通过隧道发送
		s.manager.tunnelManager.WritePacket(&packet)
	}
}

// parseSocks5UDPTarget 从SOCKS5 UDP请求中解析出目标地址
func parseSocks5UDPTarget(data []byte) (string, int) {
	if len(data) < 10 { // 最小长度 (IPv4)
		return "invalid", 0
	}

	var host string
	var port int

	addrType := data[3]
	switch addrType {
	case 0x01: // IPv4
		host = net.IP(data[4:8]).String()
		port = int(binary.BigEndian.Uint16(data[8:10]))
	case 0x03: // Domain
		domainLen := int(data[4])
		if len(data) < 5+domainLen+2 {
			return "invalid", 0
		}
		host = string(data[5 : 5+domainLen])
		port = int(binary.BigEndian.Uint16(data[5+domainLen : 5+domainLen+2]))
	default:
		return "unknown_type", 0
	}
	return host, port
}

// Close 安全地关闭会话和底层的监听器
func (s *UDPSession) Close() {
	s.closeOnce.Do(func() {
		close(s.stopChan)
		_ = s.localListener.Close()

		// 从管理器中移除自己
		s.manager.sessionMux.Lock()
		if s.manager.singletonSession == s {
			s.manager.singletonSession = nil
		}
		s.manager.sessionMux.Unlock()
	})
}

// IsRunning 检查会话是否仍在运行
func (s *UDPSession) IsRunning() bool {
	return s.running.Load()
}

// cleanupNatMap 清理过期的NAT条目
func (s *UDPSession) cleanupNatMap() {
	now := time.Now()
	s.forwardMap.Range(func(key, value interface{}) bool {
		lastSeen := value.(time.Time)
		if now.Sub(lastSeen) > udpTimeout {
			s.forwardMap.Delete(key)
		}
		return true
	})
}

// --- END OF NEW FILE ---
