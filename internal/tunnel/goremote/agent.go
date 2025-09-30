package goremote

import (
	"bufio"
	"fmt"
	"liuproxy_go/internal/shared/globalstate"
	"liuproxy_go/internal/shared/logger"
	protocol2 "liuproxy_go/internal/shared/protocol"
	"liuproxy_go/internal/shared/securecrypt"
	"net"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"

	"liuproxy_go/internal/shared/types"
)

// Tunnel 代表一个到远端的加密WebSocket连接。
type Tunnel struct {
	conn       net.Conn
	cipher     *securecrypt.Cipher
	remoteType string
}

// Agent 是 goremote 策略的核心控制器，恢复到单一、持久连接的管理模式。
type Agent struct {
	// 自身配置
	config  *types.Config
	profile *types.ServerProfile

	// 核心组件
	sessionManager *SessionManager
	udpManager     *UDPManager

	// 隧道连接管理
	currentTunnel *Tunnel
	connMutex     sync.RWMutex
	connCond      *sync.Cond
	writeMutex    sync.Mutex // 用于保护对单一隧道的并发写入
	reconnecting  atomic.Bool

	// 生命周期管理
	listener          net.Listener
	closeOnce         sync.Once
	waitGroup         sync.WaitGroup
	activeConnections atomic.Int64
}

// NewAgent 创建并组装一个完整的 goremote Agent 实例。
func NewAgent(cfg *types.Config, profile *types.ServerProfile) *Agent {
	a := &Agent{
		config:  cfg,
		profile: profile,
	}
	a.connCond = sync.NewCond(&a.connMutex)
	a.sessionManager = NewSessionManager(a)
	a.udpManager = NewUDPManager(a, cfg.CommonConf.BufferSize)
	return a
}

// Start 负责启动监听器并预连接隧道。
func (a *Agent) Start() error {
	addr := fmt.Sprintf("127.0.0.1:%d", a.profile.LocalPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("goremote agent failed to listen on %s: %w", addr, err)
	}
	a.listener = listener
	logger.Info().Str("strategy", "goremote").Str("listen_addr", a.listener.Addr().String()).Msg("Strategy listener started")

	a.waitGroup.Add(1)
	go a.acceptLoop()

	// 预连接隧道，确保启动后服务立即可用
	_, err = a.GetConnection()
	return err
}

func (a *Agent) acceptLoop() {
	defer a.waitGroup.Done()
	for {
		conn, err := a.listener.Accept()
		if err != nil {
			logger.Debug().Err(err).Msgf("[GoRemoteAgent] Listener on %s stopped accepting", a.listener.Addr())
			return
		}

		a.activeConnections.Add(1)
		a.waitGroup.Add(1)
		go func(c net.Conn) {
			defer a.waitGroup.Done()
			defer func() {
				if r := recover(); r != nil {
					logger.Error().Msgf("[GoRemoteAgent] Panic recovered in connection handler for %s: %v", c.RemoteAddr(), r)
				}
				c.Close()
				a.activeConnections.Add(-1)
			}()
			a.HandleConnection(c, bufio.NewReader(c))
		}(conn)
	}
}

// HandleConnection 分发进入的连接到 SOCKS5 或 HTTP 处理器。
func (a *Agent) HandleConnection(conn net.Conn, reader *bufio.Reader) {
	a.handleSocks5(conn, reader)
}

// GetConnection 建立或获取一个到远程服务器的连接。
func (a *Agent) GetConnection() (*Tunnel, error) {
	a.connMutex.Lock()
	defer a.connMutex.Unlock()

	for a.currentTunnel == nil {
		if a.reconnecting.Load() {
			a.connCond.Wait()
			continue
		}
		if !a.reconnecting.CompareAndSwap(false, true) {
			a.connCond.Wait()
			continue
		}

		a.connMutex.Unlock()
		var newTunnel *Tunnel
		var lastErr error
		func() {
			globalstate.GlobalStatus.Set(fmt.Sprintf("Connecting to %s...", a.profile.Remarks))
			serverCfg := a.profile
			u := url.URL{Scheme: serverCfg.Scheme, Host: net.JoinHostPort(serverCfg.Address, strconv.Itoa(serverCfg.Port)), Path: serverCfg.Path}
			conn, err := Dial(u.String())
			if err != nil {
				lastErr = err
			} else {
				cipher, cerr := securecrypt.NewCipher(a.GetConfig().Crypt)
				if cerr != nil {
					_ = conn.Close()
					lastErr = cerr
				} else {
					globalstate.GlobalStatus.Set(fmt.Sprintf("Connected to %s", serverCfg.Remarks))
					newTunnel = &Tunnel{conn: conn, cipher: cipher, remoteType: serverCfg.Type}
					go a.StartReadLoop(newTunnel)
				}
			}
			if lastErr != nil {
				failMsg := fmt.Sprintf("Failed to connect to %s: %v", a.profile.Remarks, lastErr)
				globalstate.GlobalStatus.Set(failMsg)
			}
		}()
		a.connMutex.Lock()

		a.currentTunnel = newTunnel
		a.reconnecting.Store(false)
		a.connCond.Broadcast()

		if a.currentTunnel == nil {
			return nil, fmt.Errorf("failed to connect to server %s: %w", a.profile.Remarks, lastErr)
		}
	}
	return a.currentTunnel, nil
}

// StartReadLoop 从隧道读取数据包并分发。
func (a *Agent) StartReadLoop(tunnel *Tunnel) {
	defer a.closeConnection(tunnel.conn)
	for {
		packet, err := protocol2.ReadSecurePacket(tunnel.conn, tunnel.cipher)
		if err != nil {
			return
		}
		switch packet.Flag {
		case protocol2.FlagControlNewStreamTCPSuccess:
			if s := a.sessionManager.GetSession(packet.StreamID); s != nil {
				s.SignalConnectionSuccess()
			}
		case protocol2.FlagTCPData:
			if pipe := a.sessionManager.GetStreamPipe(packet.StreamID); pipe != nil {
				pipe.PushDownstreamData(packet.Payload)
			}
		case protocol2.FlagUDPData:
			if a.udpManager != nil {
				a.udpManager.HandleDownstreamPacket(packet.Payload)
			}
		case protocol2.FlagControlCloseStream:
			a.sessionManager.RemoveStreamPipe(packet.StreamID)
			a.sessionManager.RemoveSession(packet.StreamID)
		}
	}
}

// WritePacket 将数据包写入隧道。
func (a *Agent) WritePacket(p *protocol2.Packet) error {
	tunnel, err := a.GetConnection()
	if err != nil {
		return err
	}
	a.writeMutex.Lock()
	defer a.writeMutex.Unlock()
	err = protocol2.WriteSecurePacket(tunnel.conn, p, tunnel.cipher)
	if err != nil {
		a.closeConnection(tunnel.conn)
	}
	return err
}

// closeConnection 是一个内部辅助函数，用于安全地关闭并清理隧道。
func (a *Agent) closeConnection(conn net.Conn) {
	a.connMutex.Lock()
	defer a.connMutex.Unlock()
	if a.currentTunnel != nil && a.currentTunnel.conn == conn {
		_ = a.currentTunnel.conn.Close()
		a.currentTunnel = nil
	}
}

// UpdateServerProfile 更新 Agent 使用的服务器配置。
func (a *Agent) UpdateServerProfile(profile *types.ServerProfile) {
	a.connMutex.Lock()
	defer a.connMutex.Unlock()
	logger.Info().Str("remarks", profile.Remarks).Msg("[GoRemoteAgent] Updating server profile.")
	a.profile = profile
	if a.currentTunnel != nil {
		_ = a.currentTunnel.conn.Close()
		a.currentTunnel = nil
	}
}

// Close 停止 agent、其监听器和隧道。
func (a *Agent) Close() {
	a.closeOnce.Do(func() {
		if a.listener != nil {
			logger.Info().Str("listen_addr", a.listener.Addr().String()).Msg("[GoRemoteAgent] Closing listener")
			a.listener.Close()
		}
		a.connMutex.Lock()
		if a.currentTunnel != nil {
			_ = a.currentTunnel.conn.Close()
			a.currentTunnel = nil
		}
		a.connMutex.Unlock()

		// 在等待 WaitGroup 之前，强制关闭所有活动的 SOCKS5 会话。
		// 这将导致处理这些会话的 goroutine 退出，从而使 WaitGroup 能够解锁。
		a.sessionManager.CloseAllSessions()

		a.waitGroup.Wait()
		logger.Info().Msg("[GoRemoteAgent] Agent fully stopped.")
	})
}

func (a *Agent) GetListenerAddr() net.Addr {
	if a.listener != nil {
		return a.listener.Addr()
	}
	return nil
}

func (a *Agent) GetActiveConnections() int64 {
	return a.activeConnections.Load()
}

func (a *Agent) GetCurrentBackendType() string {
	a.connMutex.RLock()
	defer a.connMutex.RUnlock()
	if a.profile != nil {
		return a.profile.Type
	}
	return ""
}

func (a *Agent) GetConfig() *types.Config { return a.config }

func (a *Agent) BuildMetadata(cmd byte, addr string) []byte { return buildMetadata(cmd, addr) }
