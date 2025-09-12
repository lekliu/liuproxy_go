// --- START OF COMPLETE REPLACEMENT for local_tunnel_manager.go ---
package socks5

import (
	"fmt"
	"liuproxy_go/internal/core/securecrypt"
	"liuproxy_go/internal/globalstate"
	"liuproxy_go/internal/protocol"
	"liuproxy_go/internal/shared"
	"log"
	"net"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

type Tunnel struct {
	conn       net.Conn
	cipher     *securecrypt.Cipher
	remoteType string
}

type TunnelManager struct {
	agent              *Agent
	SessionManager     *LocalSessionManager
	currentTunnel      *Tunnel
	connMutex          sync.RWMutex
	writeMutex         sync.Mutex
	reconnecting       atomic.Bool
	stopChan           chan struct{}
	remoteServers      [][]string
	currentServerIndex int
}

func NewTunnelManager(agent *Agent, servers [][]string) *TunnelManager {
	tm := &TunnelManager{
		agent:              agent,
		SessionManager:     NewLocalSessionManager(),
		stopChan:           make(chan struct{}),
		remoteServers:      servers,
		currentServerIndex: 0,
	}
	globalstate.GlobalStatus.Set("Idle")
	tm.SessionManager.tunnelManager = tm
	return tm
}

func (tm *TunnelManager) GetConnection() (*Tunnel, error) {
	tm.connMutex.RLock()
	if tm.currentTunnel != nil {
		tm.connMutex.RUnlock()
		return tm.currentTunnel, nil
	}
	tm.connMutex.RUnlock()

	if !tm.reconnecting.CompareAndSwap(false, true) {
		for i := 0; i < 50; i++ {
			time.Sleep(100 * time.Millisecond)
			tm.connMutex.RLock()
			if tm.currentTunnel != nil {
				tm.connMutex.RUnlock()
				return tm.currentTunnel, nil
			}
			tm.connMutex.RUnlock()
		}
		return nil, fmt.Errorf("reconnection timeout")
	}
	defer tm.reconnecting.Store(false)

	tm.connMutex.Lock()
	defer tm.connMutex.Unlock()
	if tm.currentTunnel != nil {
		return tm.currentTunnel, nil
	}

	if len(tm.remoteServers) == 0 {
		return nil, fmt.Errorf("no remote servers configured")
	}

	globalstate.GlobalStatus.Set("Connecting...")

	numServers := len(tm.remoteServers)
	var lastErr error
	for i := 0; i < numServers; i++ {
		serverIndex := (tm.currentServerIndex + i) % numServers
		serverCfg := tm.remoteServers[serverIndex]
		host, portStr, scheme, path, remoteType, edgeIP := serverCfg[0], serverCfg[1], serverCfg[2], serverCfg[3], serverCfg[4], serverCfg[5]

		u := url.URL{Scheme: scheme, Host: net.JoinHostPort(host, portStr), Path: path}

		var conn net.Conn
		var err error

		if edgeIP != "" {
			conn, err = shared.NewWebSocketConnAdapterClientWithEdgeIP(u.String(), edgeIP)
		} else {
			conn, err = shared.NewWebSocketConnAdapterClient(u.String())
		}

		if err != nil {
			lastErr = err
			continue
		}

		var algo securecrypt.Algorithm
		switch remoteType {
		case "worker":
			algo = securecrypt.AES_256_GCM
		case "remote":
			fallthrough
		default:
			algo = securecrypt.CHACHA20_POLY1305
		}

		cipher, err := securecrypt.NewCipherWithAlgo(tm.agent.config.Crypt, algo)
		if err != nil {
			_ = conn.Close()
			lastErr = err
			continue
		}

		globalstate.GlobalStatus.Set(fmt.Sprintf("Connected to %s", host))

		newTunnel := &Tunnel{conn: conn, cipher: cipher, remoteType: remoteType}
		tm.currentTunnel = newTunnel
		tm.currentServerIndex = serverIndex

		go tm.StartReadLoop(newTunnel)

		return newTunnel, nil
	}

	failMsg := fmt.Sprintf("Failed: %v", lastErr)
	globalstate.GlobalStatus.Set(failMsg)
	return nil, fmt.Errorf("all remote servers failed to connect")
}

func (tm *TunnelManager) GetCurrentBackendType() string {
	tm.connMutex.RLock()
	defer tm.connMutex.RUnlock()

	if tm.currentTunnel != nil {
		return tm.currentTunnel.remoteType
	}
	if len(tm.remoteServers) > 0 {
		serverCfg := tm.remoteServers[tm.currentServerIndex]
		return serverCfg[4]
	}
	return ""
}

func (tm *TunnelManager) WritePacket(p *protocol.Packet) error {
	tunnel, err := tm.GetConnection()
	if err != nil {
		return err
	}

	// 人为增加延时，放大并发问题，便于观察
	if p.Flag == protocol.FlagUDPData {
		time.Sleep(8 * time.Millisecond)
	}

	tm.writeMutex.Lock()
	defer tm.writeMutex.Unlock()

	// 委托给新的辅助函数进行加密和写入
	err = protocol.WriteSecurePacket(tunnel.conn, p, tunnel.cipher)
	if err != nil {
		tm.closeConnection(tunnel.conn)
	}
	return err
}

// CloseCurrentConnection 主动关闭当前隧道，用于策略重载
func (tm *TunnelManager) CloseCurrentConnection() {
	tm.connMutex.Lock()
	defer tm.connMutex.Unlock()
	if tm.currentTunnel != nil {
		log.Println("[LocalTunnel] Actively closing current tunnel for reload.")
		tm.currentTunnel.conn.Close() // 这将触发 read loop 的退出和清理
		tm.currentTunnel = nil
	}
}

// UpdateRemoteServers 允许从外部更新远程服务器列表
func (tm *TunnelManager) UpdateRemoteServers(servers [][]string) {
	tm.connMutex.Lock()
	defer tm.connMutex.Unlock()
	log.Println("[LocalTunnel] Updating remote server list.")
	tm.remoteServers = servers
	tm.currentServerIndex = 0 // 重置索引，以便从新的列表开始
}

func (tm *TunnelManager) closeConnection(conn net.Conn) {
	tm.connMutex.Lock()
	defer tm.connMutex.Unlock()
	if tm.currentTunnel != nil && tm.currentTunnel.conn == conn {
		_ = tm.currentTunnel.conn.Close()
		tm.currentTunnel = nil
	}
}

func (tm *TunnelManager) StartReadLoop(tunnel *Tunnel) {
	defer func() {
		tm.closeConnection(tunnel.conn)
		tm.SessionManager.CloseAll()
	}()
	for {
		// 委托给新的辅助函数进行读取和解密
		packet, err := protocol.ReadSecurePacket(tunnel.conn, tunnel.cipher)
		if err != nil {
			return
		}

		switch packet.Flag {
		case protocol.FlagControlNewStreamTCPSuccess:
			if s := tm.SessionManager.GetSession(packet.StreamID); s != nil {
				s.SignalConnectionSuccess()
			}
		case protocol.FlagTCPData:
			if pipe := tm.SessionManager.GetStreamPipe(packet.StreamID); pipe != nil {
				pipe.PushDownstreamData(packet.Payload)
			}
		case protocol.FlagUDPData:
			if tm.agent != nil && tm.agent.udpManager != nil {
				tm.agent.udpManager.HandleDownstreamPacket(packet.Payload)
			}
		case protocol.FlagControlCloseStream:
			tm.SessionManager.RemoveStreamPipe(packet.StreamID)
			tm.SessionManager.RemoveSession(packet.StreamID)
		}
	}
}
