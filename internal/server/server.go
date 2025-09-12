// --- START OF COMPLETE REPLACEMENT for server.go ---
package server

import (
	"fmt"
	"liuproxy_go/internal/config"
	"liuproxy_go/internal/types"
	"liuproxy_go/internal/web"
	"log"
	"net"
	"sync"
	"time"

	"liuproxy_go/internal/agent/socks5"
)

var (
	globalAppServer *AppServer
	globalMutex     sync.RWMutex
)

// GetGlobalServer provides safe, concurrent access to the global AppServer instance.
func GetGlobalServer() *AppServer {
	globalMutex.RLock()
	defer globalMutex.RUnlock()
	return globalAppServer
}

// ManagedListener holds a listener and its type for identification purposes.
type ManagedListener struct {
	net.Listener
	PortType string
}

// AppServer 是应用的主结构体，持有配置和核心组件
type AppServer struct {
	cfg                *types.Config
	configPath         string
	strategy           socks5.TunnelStrategy
	tunnelManager      *socks5.TunnelManager
	udpManager         *socks5.UDPManager
	httpHandler        *socks5.HTTPHandler
	socks5Handler      *socks5.Agent // FIX: Reverted to concrete type to allow method access
	listeners          []*ManagedListener
	waitGroup          sync.WaitGroup
	stopOnce           sync.Once
	isMobile           bool
	mobileListenerChan chan net.Listener
	mobileErrChan      chan error
}

// New 创建一个新的 AppServer 实例
func New(cfg *types.Config, configPath string) *AppServer {
	appSrv := &AppServer{
		cfg:                cfg,
		configPath:         configPath,
		listeners:          make([]*ManagedListener, 0),
		mobileListenerChan: make(chan net.Listener, 1),
		mobileErrChan:      make(chan error, 1),
	}
	globalMutex.Lock()
	globalAppServer = appSrv
	globalMutex.Unlock()
	return appSrv
}

// parseProfilesFromConfig centralizes the logic of converting ini strings to BackendProfile structs.
func parseProfilesFromConfig(remoteIPs [][]string) []*socks5.BackendProfile {
	var profiles []*socks5.BackendProfile
	for _, remoteCfg := range remoteIPs {
		if len(remoteCfg) < 8 {
			log.Printf("[AppServer] WARNING: Skipping malformed remote_ip config with less than 8 fields.")
			continue
		}
		profiles = append(profiles, &socks5.BackendProfile{
			Remarks: remoteCfg[0],
			Address: remoteCfg[1],
			Port:    remoteCfg[2],
			Scheme:  remoteCfg[3],
			Path:    remoteCfg[4],
			Type:    remoteCfg[5],
			EdgeIP:  remoteCfg[6],
		})
	}
	return profiles
}

// Run 是服务器的启动入口 (用于命令行)
// 它会启动所有服务并阻塞直到所有服务都停止。
func (s *AppServer) Run() {
	log.Printf("Starting server in '%s' mode...", s.cfg.Mode)
	if s.cfg.Mode == "local" {
		s.runLocal()
		// 启动 Web 服务器
		web.StartServer(&s.waitGroup, s.cfg, s.configPath, s.ReloadStrategy)
	} else if s.cfg.Mode == "remote" {
		s.runRemote()
	} else {
		log.Fatalf("Unknown mode: %s", s.cfg.Mode)
	}

	s.Wait()
}

// RunMobile 是服务器的启动入口 (用于移动端)
// 它启动所有服务但不阻塞，并返回创建的监听器。
func (s *AppServer) RunMobile() (*ManagedListener, error) {
	log.Printf("Starting mobile server in '%s' mode...", s.cfg.Mode)
	if s.cfg.Mode != "local" {
		return nil, fmt.Errorf("RunMobile is only supported in 'local' mode")
	}

	s.isMobile = true
	go s.runLocal() // runLocal will send listener or error to channels

	select {
	case listener := <-s.mobileListenerChan:
		// Find the managed listener that corresponds to this one
		for _, ml := range s.listeners {
			if ml.Listener == listener {
				return ml, nil
			}
		}
		return nil, fmt.Errorf("internal error: listener not found in managed list")
	case err := <-s.mobileErrChan:
		return nil, err
	case <-time.After(10 * time.Second):
		return nil, fmt.Errorf("timeout waiting for listener to start")
	}
}

// Wait 会阻塞直到所有服务器的 goroutine 都已退出。
func (s *AppServer) Wait() {
	s.waitGroup.Wait()
	log.Println("All server routines have finished.")
}

// Stop 会优雅地关闭所有正在运行的监听器。
func (s *AppServer) Stop() {
	s.stopOnce.Do(func() {
		log.Println("Stopping server...")
		for _, l := range s.listeners {
			log.Printf("Closing listener for '%s' on %s", l.PortType, l.Listener.Addr())
			l.Listener.Close()
		}
	})
}

func (s *AppServer) ReloadStrategy() error {
	log.Println("[AppServer] Reloading strategy due to configuration change...")
	if err := config.LoadIni(s.cfg, s.configPath); err != nil {
		log.Printf("!!! FAILED to reload config file '%s': %v", s.configPath, err)
		return err
	}

	profiles := parseProfilesFromConfig(s.cfg.LocalConf.RemoteIPs)

	newStrategy, err := socks5.CreateStrategyFromConfig(s.cfg, profiles)
	if err != nil {
		log.Printf("!!! FAILED to create new strategy on reload: %v", err)
		return err
	}

	if grs, ok := s.strategy.(*socks5.GoRemoteStrategy); ok {
		grs.CloseTunnel()
		if newGrs, ok := newStrategy.(*socks5.GoRemoteStrategy); ok {
			newGrs.UpdateServers(profiles)
		}
	}

	s.strategy = newStrategy
	log.Printf(">>> Strategy successfully reloaded. New active strategy: '%s'", s.strategy.GetType())

	// Initialize new strategy in the background
	go func() {
		err := s.strategy.Initialize()
		// If the background initialization fails, we need to update the status.
		// We can only do this for GoRemoteStrategy as it's the only one with a shared TunnelManager.
		if err != nil {
			log.Printf("[AppServer] Error during background initialization of new strategy: %v", err)
			if _, ok := s.strategy.(*socks5.GoRemoteStrategy); ok {
				// The GetConnection method inside Initialize already sets the failure status.
				// No extra action is needed here, the log is sufficient.
			}
		}
	}()

	return nil
}
