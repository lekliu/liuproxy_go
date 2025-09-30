// --- START OF COMPLETE REPLACEMENT for liuproxy_go/internal/goremote/udp_session.go ---
package goremote

import (
	"encoding/binary"
	"liuproxy_go/internal/shared/protocol"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

const udpTimeout = 60 * time.Second

type UDPSession struct {
	agent         *Agent
	localListener net.PacketConn
	forwardMap    sync.Map
	reverseMap    sync.Map
	closeOnce     sync.Once
	stopChan      chan struct{}
	running       atomic.Bool
}

func (s *UDPSession) GetListenerAddr() net.Addr {
	return s.localListener.LocalAddr()
}

func (s *UDPSession) runUpstreamLoop() {
	s.running.Store(true)
	defer s.running.Store(false)
	defer s.Close()

	buf := make([]byte, s.agent.udpManager.bufferSize)
	for {
		_ = s.localListener.SetReadDeadline(time.Now().Add(udpTimeout))
		n, clientAddr, err := s.localListener.ReadFrom(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				select {
				case <-s.stopChan:
					return
				default:
					s.cleanupNatMap()
					continue
				}
			}
			return
		}
		targetHost, targetPort := parseSocks5UDPTarget(buf[:n])
		targetAddrStr := net.JoinHostPort(targetHost, strconv.Itoa(targetPort))
		clientAddrStr := clientAddr.String()
		s.forwardMap.Store(clientAddrStr, time.Now())
		s.reverseMap.Store(targetAddrStr, clientAddrStr)
		payloadCopy := make([]byte, n)
		copy(payloadCopy, buf[:n])
		packet := protocol.Packet{
			StreamID: 0xFFFF,
			Flag:     protocol.FlagUDPData,
			Payload:  payloadCopy,
		}
		s.agent.WritePacket(&packet)
	}
}

func parseSocks5UDPTarget(data []byte) (string, int) {
	if len(data) < 10 {
		return "invalid", 0
	}
	var host string
	var port int
	addrType := data[3]
	switch addrType {
	case 0x01:
		host = net.IP(data[4:8]).String()
		port = int(binary.BigEndian.Uint16(data[8:10]))
	case 0x03:
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

func (s *UDPSession) Close() {
	s.closeOnce.Do(func() {
		close(s.stopChan)
		_ = s.localListener.Close()
		s.agent.udpManager.sessionMux.Lock()
		if s.agent.udpManager.singletonSession == s {
			s.agent.udpManager.singletonSession = nil
		}
		s.agent.udpManager.sessionMux.Unlock()
	})
}

func (s *UDPSession) IsRunning() bool {
	return s.running.Load()
}

func (s *UDPSession) cleanupNatMap() {
	now := time.Now()
	s.forwardMap.Range(func(key, value interface{}) bool {
		if now.Sub(value.(time.Time)) > udpTimeout {
			s.forwardMap.Delete(key)
		}
		return true
	})
}

// --- END OF COMPLETE REPLACEMENT ---
