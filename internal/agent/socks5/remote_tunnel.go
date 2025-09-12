// --- START OF COMPLETE REPLACEMENT for remote_tunnel.go ---
package socks5

import (
	"liuproxy_go/internal/core/securecrypt"
	"net"
	"sync"

	"liuproxy_go/internal/protocol"
)

type RemoteTunnel struct {
	conn           net.Conn
	agent          *Agent
	sessionManager *RemoteSessionManager
	closeOnce      sync.Once
	writeMutex     sync.Mutex
	cipher         *securecrypt.Cipher
}

func NewRemoteTunnel(conn net.Conn, agent *Agent) *RemoteTunnel {
	cipher, err := securecrypt.NewCipher(agent.config.Crypt)
	if err != nil {
		return nil
	}
	return &RemoteTunnel{
		conn:           conn,
		agent:          agent,
		sessionManager: NewRemoteSessionManager(),
		cipher:         cipher,
	}
}

func (t *RemoteTunnel) StartReadLoop() {
	defer t.Close()
	for {
		// 委托给新的辅助函数进行读取和解密
		packet, err := protocol.ReadSecurePacket(t.conn, t.cipher)
		if err != nil {
			break
		}

		switch packet.Flag {
		case protocol.FlagControlNewStreamTCP:
			go t.sessionManager.NewTCPSession(packet.StreamID, packet.Payload, t)
		case protocol.FlagTCPData:
			if s := t.sessionManager.GetSession(packet.StreamID); s != nil {
				s.WriteToTarget(packet.Payload)
			}
		case protocol.FlagUDPData:
			if t.agent.remoteUdpRelay != nil {
				go t.agent.remoteUdpRelay.HandlePacketFromTunnel(packet, t)
			}
		case protocol.FlagControlCloseStream:
			t.sessionManager.RemoveSession(packet.StreamID)
		}
	}
}

func (t *RemoteTunnel) WritePacket(p *protocol.Packet) error {
	t.writeMutex.Lock()
	defer t.writeMutex.Unlock()

	// 委托给新的辅助函数进行加密和写入
	err := protocol.WriteSecurePacket(t.conn, p, t.cipher)
	if err != nil {
		t.Close()
	}
	return err
}

func (t *RemoteTunnel) Close() {
	t.closeOnce.Do(func() {
		t.sessionManager.CloseAll()
		_ = t.conn.Close()
	})
}
