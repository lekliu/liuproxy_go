package goremote

import (
	"fmt"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"net"

	"liuproxy_go/internal/shared/types"
)

type GoRemoteStrategy struct {
	agent  *Agent
	logger zerolog.Logger
}

var _ types.TunnelStrategy = (*GoRemoteStrategy)(nil)

// NewGoRemoteStrategy 现在只传递必要的 cfg 和 profile。
func NewGoRemoteStrategy(cfg *types.Config, profile *types.ServerProfile) (types.TunnelStrategy, error) {
	agent := NewAgent(cfg, profile)
	return &GoRemoteStrategy{
		agent: agent,
		logger: log.With().
			Str("strategy_type", "goremote").
			Str("server_id", profile.ID).
			Str("remarks", profile.Remarks).Logger(),
	}, nil
}

func (s *GoRemoteStrategy) GetMetrics() *types.Metrics {
	return &types.Metrics{
		ActiveConnections: s.agent.GetActiveConnections(),
	}
}

func (s *GoRemoteStrategy) Initialize() error {
	err := s.agent.Start()
	if err == nil {
		s.logger.Info().
			Str("listen_addr", s.agent.GetListenerAddr().String()).
			Msg("Strategy listener started")
	}
	return err
}

func (s *GoRemoteStrategy) GetListenerInfo() *types.ListenerInfo {
	addr := s.agent.GetListenerAddr()
	if addr == nil {
		return nil
	}
	tcpAddr, ok := addr.(*net.TCPAddr)
	if !ok {
		return nil // Should not happen for this strategy
	}
	return &types.ListenerInfo{
		Address: tcpAddr.IP.String(),
		Port:    tcpAddr.Port,
	}
}

func (s *GoRemoteStrategy) GetType() string {
	return "goremote"
}

func (s *GoRemoteStrategy) CloseTunnel() {
	s.agent.Close()
}

func (s *GoRemoteStrategy) UpdateServer(profile *types.ServerProfile) error {
	if s.agent != nil {
		s.agent.UpdateServerProfile(profile)
		return nil
	}
	return fmt.Errorf("agent not initialized in GoRemoteStrategy")
}

// CheckHealth for GoRemoteStrategy tries to get or establish the persistent tunnel.
func (s *GoRemoteStrategy) CheckHealth() error {
	_, err := s.agent.GetConnection()
	return err
}
