// *********** START OF COMPLETE REPLACEMENT for internal/strategy/vless_strategy_native.go ***********
package vless

import (
	"bufio"
	"context"
	"fmt"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"liuproxy_go/internal/shared/logger"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"liuproxy_go/internal/shared/types"
)

// VlessStrategyNative 现在是一个无状态的监听器和分发器。
type VlessStrategyNative struct {
	config            *types.Config
	profile           *types.ServerProfile
	listener          net.Listener
	listenerInfo      *types.ListenerInfo
	closeOnce         sync.Once
	waitGroup         sync.WaitGroup
	activeConnections atomic.Int64
	logger            zerolog.Logger
	activeConns       sync.Map // 用于追踪所有活跃的客户端连接
}

// NewVlessStrategy 是一个工厂函数，根据配置选择使用 xray-core 还是原生实现
func NewVlessStrategy(cfg *types.Config, profile *types.ServerProfile) (types.TunnelStrategy, error) {
	logger.Info().Str("implementation", "native").Msg("Creating VLESS strategy")

	// 检查 profile 是否有效
	if profile == nil {
		return nil, fmt.Errorf("vless strategy requires a non-nil profile")
	}

	// 总是调用原生实现
	return NewVlessStrategyNative(cfg, profile)
}

// NewVlessStrategyNative 创建一个新的 VLESS 原生策略实例。
func NewVlessStrategyNative(cfg *types.Config, profile *types.ServerProfile) (types.TunnelStrategy, error) {
	return &VlessStrategyNative{
		config:  cfg,
		profile: profile,
		logger: log.With().
			Str("strategy_type", "vless-native").
			Str("server_id", profile.ID).
			Str("remarks", profile.Remarks).Logger(),
	}, nil
}

func (s *VlessStrategyNative) Initialize() error {
	addr := fmt.Sprintf("127.0.0.1:%d", s.profile.LocalPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("native vless: failed to listen on %s: %w", addr, err)
	}
	s.listener = listener

	tcpAddr := s.listener.Addr().(*net.TCPAddr)
	s.listenerInfo = &types.ListenerInfo{
		Address: tcpAddr.IP.String(),
		Port:    tcpAddr.Port,
	}

	s.logger.Info().Str("listen_addr", s.listener.Addr().String()).Msg("Strategy listener started")

	s.waitGroup.Add(1)
	go s.acceptLoop()

	return nil
}

func (s *VlessStrategyNative) acceptLoop() {
	defer s.waitGroup.Done()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			s.logger.Debug().Err(err).Msgf("Listener on %s stopped accepting", s.listener.Addr())
			return
		}
		s.activeConns.Store(conn, struct{}{}) // 注册连接
		s.waitGroup.Add(1)
		go s.handleClientConnection(conn)
	}
}

func (s *VlessStrategyNative) handleClientConnection(clientConn net.Conn) {
	defer s.activeConns.Delete(clientConn) // 注销连接
	s.activeConnections.Add(1)
	defer s.waitGroup.Done()
	defer s.activeConnections.Add(-1)

	// 将子 logger 注入到 vless 包的处理函数中
	ctx := s.logger.WithContext(context.Background())
	HandleConnection(ctx, clientConn, bufio.NewReader(clientConn), s.profile)
}

// HandleConnection 现在是一个纯粹的分发器，根据网络类型选择处理器。
func (s *VlessStrategyNative) HandleConnection(clientConn net.Conn, reader *bufio.Reader) {
	// This method is kept for compatibility but should be deprecated.
	// New connections are handled by handleClientConnection.
	s.logger.Warn().Msg("HandleConnection called directly, this is deprecated.")
	ctx := s.logger.WithContext(context.Background())
	HandleConnection(ctx, clientConn, reader, s.profile)
}

func (s *VlessStrategyNative) GetType() string { return "vless" }

func (s *VlessStrategyNative) CloseTunnel() {
	s.closeOnce.Do(func() {
		if s.listener != nil {
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
	})
}

func (s *VlessStrategyNative) GetListenerInfo() *types.ListenerInfo { return s.listenerInfo }

func (s *VlessStrategyNative) GetMetrics() *types.Metrics {
	return &types.Metrics{ActiveConnections: s.activeConnections.Load()}
}

func (s *VlessStrategyNative) UpdateServer(profile *types.ServerProfile) error {
	s.profile = profile
	s.logger = log.With().
		Str("strategy_type", "vless-native").
		Str("server_id", profile.ID).
		Str("remarks", profile.Remarks).Logger()
	// 由于现在是每个请求都创建新连接，所以更新 profile 会立即对下一个新连接生效。
	s.logger.Info().Msg("VLESS native profile updated. New settings will apply to subsequent connections.")
	return nil
}

// CheckHealth for VlessStrategyNative performs a real connection attempt to the remote VLESS server.
func (s *VlessStrategyNative) CheckHealth() error {
	var conn net.Conn
	var err error

	network := s.profile.Network
	if network == "" {
		network = "ws" // Default to ws
	}

	// We need to call the internal dialer functions from the vless package.
	// Since they are not exported, we need to temporarily move this logic into the vless package
	// or export them. For now, let's assume we can call them.
	// Let's re-implement the dial logic here for simplicity.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	switch network {
	case "grpc":
		conn, err = DialVlessGRPC(ctx, s.profile)
	case "ws":
		conn, err = DialVlessWS(ctx, s.profile)
	default:
		err = fmt.Errorf("unsupported network type for health check: %s", network)
	}

	if err != nil {
		return err
	}

	conn.Close()
	return nil
}
