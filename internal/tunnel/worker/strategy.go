package worker

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"liuproxy_go/internal/shared/globalstate"
	protocol2 "liuproxy_go/internal/shared/protocol"
	"liuproxy_go/internal/shared/securecrypt"
	"net"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"liuproxy_go/internal/shared/types"
)

type WorkerStrategy struct {
	config            *types.Config
	profile           *types.ServerProfile
	listener          net.Listener
	listenerInfo      *types.ListenerInfo
	closeOnce         sync.Once
	waitGroup         sync.WaitGroup
	activeConnections atomic.Int64
	logger            zerolog.Logger
	activeConns       sync.Map
}

// Ensure WorkerStrategy implements TunnelStrategy interface
var _ types.TunnelStrategy = (*WorkerStrategy)(nil)

func NewWorkerStrategy(cfg *types.Config, profile *types.ServerProfile) (types.TunnelStrategy, error) {
	return &WorkerStrategy{
		config:  cfg,
		profile: profile,
		logger: log.With().
			Str("strategy_type", "worker").
			Str("server_id", profile.ID).
			Str("remarks", profile.Remarks).Logger(),
	}, nil
}

func (s *WorkerStrategy) Initialize() error {
	addr := fmt.Sprintf("127.0.0.1:%d", s.profile.LocalPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("worker strategy failed to listen on %s: %w", addr, err)
	}
	s.listener = listener

	tcpAddr := s.listener.Addr().(*net.TCPAddr)
	s.listenerInfo = &types.ListenerInfo{
		Address: tcpAddr.IP.String(),
		Port:    tcpAddr.Port,
	}

	s.logger.Info().Str("strategy", "worker").Str("listen_addr", s.listener.Addr().String()).Msg("Strategy listener started")

	s.waitGroup.Add(1)
	go s.acceptLoop()

	return nil
}

func (s *WorkerStrategy) acceptLoop() {
	defer s.waitGroup.Done()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			s.logger.Debug().Err(err).Msgf("[WorkerStrategy] Listener on %s stopped accepting connections", s.listener.Addr())
			return
		}

		s.activeConns.Store(conn, struct{}{}) // 注册连接
		s.activeConnections.Add(1)            // 增加计数器
		s.waitGroup.Add(1)
		go func(c net.Conn) {
			defer s.waitGroup.Done()
			defer func() {
				if r := recover(); r != nil {
					s.logger.Error().Msgf("[WorkerStrategy] Panic recovered in connection handler for %s: %v", c.RemoteAddr(), r)
				}
				c.Close()
				s.activeConnections.Add(-1) // 减少计数器
				s.activeConns.Delete(c)
			}()
			s.handleClientConnection(c)
		}(conn)
	}
}

func (s *WorkerStrategy) handleClientConnection(plainConn net.Conn) {
	reader := bufio.NewReader(plainConn)
	agent := NewAgent(s.config)
	cmd, targetAddr, err := agent.HandshakeWithClient(plainConn, reader)
	if err != nil {
		s.logger.Debug().Err(err).Str("client_ip", plainConn.RemoteAddr().String()).Msg("Worker SOCKS5 handshake failed")
		return
	}

	if cmd != 1 { // 1 = CONNECT
		if cmd == 3 { // 3 = UDP ASSOCIATE, Worker不支持
			// 明确回复不支持UDP
			_, _ = plainConn.Write([]byte{0x05, 0x08, 0x00, 0x01, 0, 0, 0, 0, 0, 0}) // Address type not supported
		}
		s.logger.Debug().Int("command", int(cmd)).Msg("Worker received unsupported SOCKS5 command")
		return
	}

	tunnelConn, cipher, err := s.createTunnel()
	if err != nil {
		s.logger.Error().Err(err).Msg("[WorkerStrategy] Failed to create tunnel")
		return
	}
	defer tunnelConn.Close()

	const streamID uint16 = 1
	packet := protocol2.Packet{
		StreamID: streamID,
		Flag:     protocol2.FlagControlNewStreamTCP,
		Payload:  buildMetadataForWorker(1, targetAddr),
	}

	if err := protocol2.WriteSecurePacket(tunnelConn, &packet, cipher); err != nil {
		s.logger.Error().Err(err).Msg("[WorkerStrategy] Failed to write NewStream request")
		return
	}

	if err := s.waitForSuccess(tunnelConn); err != nil {
		s.logger.Error().Err(err).Msg("[WorkerStrategy] Did not receive success from worker")
		return
	}

	_, _ = plainConn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})

	globalstate.GlobalStatus.Set(fmt.Sprintf("Connected (Worker via %s)", s.profile.Address))

	var wg sync.WaitGroup
	wg.Add(2)

	// Upstream (Client -> Worker): Encrypted
	go func() {
		defer wg.Done()
		defer tunnelConn.Close()
		buf := make([]byte, s.config.CommonConf.BufferSize)
		for {
			n, err := plainConn.Read(buf)
			if err != nil {
				closePacket := protocol2.Packet{StreamID: streamID, Flag: protocol2.FlagControlCloseStream}
				_ = protocol2.WriteSecurePacket(tunnelConn, &closePacket, cipher)
				return
			}
			packet := protocol2.Packet{StreamID: streamID, Flag: protocol2.FlagTCPData, Payload: buf[:n]}
			if err := protocol2.WriteSecurePacket(tunnelConn, &packet, cipher); err != nil {
				return
			}
		}
	}()

	// Downstream (Worker -> Client): Unencrypted
	go func() {
		defer wg.Done()
		defer plainConn.Close()
		for {
			packet, err := protocol2.ReadUnsecurePacket(tunnelConn)
			if err != nil {
				return
			}
			if packet.Flag == protocol2.FlagTCPData {
				if _, err := plainConn.Write(packet.Payload); err != nil {
					return
				}
			} else if packet.Flag == protocol2.FlagControlCloseStream {
				return
			}
		}
	}()
	wg.Wait()
}

func (s *WorkerStrategy) GetListenerInfo() *types.ListenerInfo {
	return s.listenerInfo
}

func (s *WorkerStrategy) GetMetrics() *types.Metrics {
	return &types.Metrics{
		ActiveConnections: s.activeConnections.Load(),
	}
}

func (s *WorkerStrategy) GetType() string {
	return "worker"
}

func (s *WorkerStrategy) CloseTunnel() {
	s.closeOnce.Do(func() {
		if s.listener != nil {
			s.logger.Info().Str("listen_addr", s.listener.Addr().String()).Msg("[WorkerStrategy] Closing listener")
			s.listener.Close()
		}
		// 在等待 WaitGroup 之前，强制关闭所有活动的连接
		s.activeConns.Range(func(key, value interface{}) bool {
			if conn, ok := key.(net.Conn); ok {
				conn.Close()
			}
			return true
		})
		s.waitGroup.Wait()
		s.logger.Info().Msg("[WorkerStrategy] Listener and all connections closed.")
	})
}

func (s *WorkerStrategy) UpdateServer(profile *types.ServerProfile) error {
	// Worker strategy connections are short-lived, typically no need for hot-update.
	// The AppServer's ReloadStrategy is sufficient.
	return fmt.Errorf("worker strategy does not support hot-update")
}

// CheckHealth for WorkerStrategy performs a real connection attempt to the remote worker.
func (s *WorkerStrategy) CheckHealth() error {
	s.logger.Debug().Msg("WorkerStrategy.CheckHealth: Attempting to create tunnel for health check...")
	conn, _, err := s.createTunnel()
	if err != nil {
		s.logger.Warn().Err(err).Msg("WorkerStrategy.CheckHealth: Failed.")
		return err
	}
	// We must close the connection immediately as it's just for a health check.
	conn.Close()
	s.logger.Debug().Msg("WorkerStrategy.CheckHealth: Passed.")
	return nil
}

func (s *WorkerStrategy) createTunnel() (net.Conn, *securecrypt.Cipher, error) {
	u := url.URL{
		Scheme: s.profile.Scheme,
		Host:   net.JoinHostPort(s.profile.Address, strconv.Itoa(s.profile.Port)),
		Path:   s.profile.Path,
	}
	tunnelConn, err := Dial(u.String(), s.profile.EdgeIP)
	if err != nil {
		return nil, nil, err
	}
	cipher, err := securecrypt.NewCipherWithAlgo(s.config.CommonConf.Crypt, securecrypt.AES_256_GCM)
	if err != nil {
		_ = tunnelConn.Close()
		return nil, nil, err
	}
	return tunnelConn, cipher, nil
}

// waitForSuccess expects an UNENCRYPTED success packet from the worker.
func (s *WorkerStrategy) waitForSuccess(conn net.Conn) error {
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	defer conn.SetReadDeadline(time.Time{})
	packet, err := protocol2.ReadUnsecurePacket(conn)
	if err != nil {
		return err
	}
	if packet.Flag != protocol2.FlagControlNewStreamTCPSuccess {
		return fmt.Errorf("unexpected flag from worker: got %d, want %d", packet.Flag, protocol2.FlagControlNewStreamTCPSuccess)
	}
	return nil
}

func buildMetadataForWorker(cmd byte, targetAddr string) []byte {
	host, portStr, _ := net.SplitHostPort(targetAddr)
	port, _ := strconv.Atoi(portStr)
	addrBytes := []byte(host)
	addrType := byte(0x03)
	if ip := net.ParseIP(host); ip != nil {
		if ipv4 := ip.To4(); ipv4 != nil {
			addrType = 0x01
			addrBytes = ipv4
		} else {
			addrType = 0x04
			addrBytes = ip.To16()
		}
	}
	var buf bytes.Buffer
	buf.WriteByte(cmd)
	buf.WriteByte(addrType)
	if addrType == 0x03 {
		buf.WriteByte(byte(len(addrBytes)))
	}
	buf.Write(addrBytes)
	_ = binary.Write(&buf, binary.BigEndian, uint16(port))
	return buf.Bytes()
}
