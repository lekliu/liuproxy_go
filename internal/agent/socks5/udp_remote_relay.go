// --- START OF COMPLETE REPLACEMENT for udp_remote_relay.go ---
package socks5

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"time"

	"liuproxy_go/internal/core/securecrypt"
	"liuproxy_go/internal/protocol"
	"liuproxy_go/internal/types"
)

type udpSession struct {
	conn     net.Conn
	lastSeen time.Time
}
type RemoteUDPRelay struct {
	sessions    map[string]*udpSession
	lock        sync.Mutex
	bufferSize  int
	dnsResolver *DNSResolver
	stopChan    chan struct{}
	once        sync.Once
}

func NewRemoteUDPRelay(cfg types.Config, bufferSize int) (*RemoteUDPRelay, error) {
	// RemoteUDPRelay自身不再直接使用cipher，但DNSResolver需要，所以保留创建逻辑
	cipher, err := securecrypt.NewCipher(cfg.Crypt)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher for dns resolver: %w", err)
	}
	return &RemoteUDPRelay{
		sessions:    make(map[string]*udpSession),
		bufferSize:  bufferSize,
		dnsResolver: NewDNSResolver(cipher), // DNSResolver仍然需要cipher
		stopChan:    make(chan struct{}),
	}, nil
}

func (r *RemoteUDPRelay) startCleanupTask() {
	r.once.Do(func() {
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					r.cleanupExpiredSessions()
				case <-r.stopChan:
					return
				}
			}
		}()
	})
}

func (r *RemoteUDPRelay) cleanupExpiredSessions() {
	r.lock.Lock()
	defer r.lock.Unlock()

	now := time.Now()
	for key, session := range r.sessions {
		if now.Sub(session.lastSeen) > udpTimeout {
			session.conn.Close()
			delete(r.sessions, key)
		}
	}
}

// HandlePacketFromTunnel 接收由RemoteTunnel解密后的明文SOCKS5 UDP包
func (r *RemoteUDPRelay) HandlePacketFromTunnel(packet *protocol.Packet, tunnel *RemoteTunnel) {
	r.startCleanupTask()

	decryptedPayload := packet.Payload

	isDNS, _, udpPayloadForDNS := isDNSRequest(decryptedPayload)
	if isDNS {
		// DNSResolver的响应会通过tunnel.WritePacket自动加密
		go r.dnsResolver.HandleDNSRequest(udpPayloadForDNS, tunnel, packet.StreamID, decryptedPayload)
		return
	}

	if len(decryptedPayload) < 4 || decryptedPayload[2] != 0x00 {
		return
	}

	var targetAddr string
	var udpPayload []byte
	var socksHeader []byte

	addrType := decryptedPayload[3]

	switch addrType {
	case 0x01: // IPv4
		if len(decryptedPayload) < 10 {
			return
		}
		targetHost := net.IP(decryptedPayload[4:8]).String()
		targetPort := int(binary.BigEndian.Uint16(decryptedPayload[8:10]))
		targetAddr = fmt.Sprintf("%s:%d", targetHost, targetPort)
		socksHeader = decryptedPayload[0:10]
		udpPayload = decryptedPayload[10:]

	case 0x03: // Domain Name
		domainLen := int(decryptedPayload[4])
		headerLen := 5 + domainLen + 2
		if len(decryptedPayload) < headerLen {
			return
		}
		targetHost := string(decryptedPayload[5 : 5+domainLen])
		targetPort := int(binary.BigEndian.Uint16(decryptedPayload[5+domainLen : headerLen]))
		targetAddr = fmt.Sprintf("%s:%d", targetHost, targetPort)
		socksHeader = decryptedPayload[0:headerLen]
		udpPayload = decryptedPayload[headerLen:]

	default:
		return
	}

	sessionKey := fmt.Sprintf("%s-%d", tunnel.conn.RemoteAddr().String(), packet.StreamID)

	r.lock.Lock()
	session, found := r.sessions[sessionKey]
	if !found {
		conn, dialErr := net.DialTimeout("udp", targetAddr, 10*time.Second)
		if dialErr != nil {
			r.lock.Unlock()
			return
		}
		session = &udpSession{
			conn:     conn,
			lastSeen: time.Now(),
		}
		r.sessions[sessionKey] = session
		go r.copyFromTargetToTunnel(session.conn, sessionKey, tunnel, packet.StreamID, socksHeader)
	}

	session.lastSeen = time.Now()
	targetConn := session.conn
	r.lock.Unlock()

	_, _ = targetConn.Write(udpPayload)
	_ = targetConn.SetReadDeadline(time.Now().Add(udpTimeout))
}

// copyFromTargetToTunnel 从目标服务器读取明文数据，封装成明文SOCKS5 UDP包，
// 然后交由隧道发送（隧道会负责加密）。
func (r *RemoteUDPRelay) copyFromTargetToTunnel(targetConn net.Conn, sessionKey string, tunnel *RemoteTunnel, streamID uint16, originalSocksHeader []byte) {
	defer func() {
		r.lock.Lock()
		if s, ok := r.sessions[sessionKey]; ok && s.conn == targetConn {
			delete(r.sessions, sessionKey)
		}
		targetConn.Close()
		r.lock.Unlock()
	}()

	buf := make([]byte, r.bufferSize)
	for {
		n, err := targetConn.Read(buf)
		if err != nil {
			return
		}

		r.lock.Lock()
		if session, ok := r.sessions[sessionKey]; ok {
			session.lastSeen = time.Now()
		}
		r.lock.Unlock()

		// 构造明文SOCKS5 UDP响应包
		responsePacket := append(originalSocksHeader, buf[:n]...)

		packet := protocol.Packet{
			StreamID: streamID,
			Flag:     protocol.FlagUDPData,
			Payload:  responsePacket, // Payload是明文
		}

		if err := tunnel.WritePacket(&packet); err != nil {
			return
		}
	}
}

func isDNSRequest(decryptedPayload []byte) (isDNS bool, targetAddr string, payload []byte) {
	if len(decryptedPayload) < 10 || decryptedPayload[2] != 0x00 {
		return false, "", nil
	}

	var host string
	var port int
	var dataOffset int

	addrType := decryptedPayload[3]
	switch addrType {
	case 0x01:
		host = net.IP(decryptedPayload[4:8]).String()
		port = int(binary.BigEndian.Uint16(decryptedPayload[8:10]))
		dataOffset = 10
	case 0x03:
		domainLen := int(decryptedPayload[4])
		if len(decryptedPayload) < 5+domainLen+2 {
			return false, "", nil
		}
		host = string(decryptedPayload[5 : 5+domainLen])
		port = int(binary.BigEndian.Uint16(decryptedPayload[5+domainLen : 5+domainLen+2]))
		dataOffset = 5 + domainLen + 2
	default:
		return false, "", nil
	}

	if port == 53 && host == "172.16.0.2" {
		return true, net.JoinHostPort(host, "53"), decryptedPayload[dataOffset:]
	}
	return false, "", nil
}
