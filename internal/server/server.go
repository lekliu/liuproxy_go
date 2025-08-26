// 修改点 1: 导入 "fmt" 包以使用打印功能 **********
// Modification Point 1: Import the "fmt" package to use printing functions **********
// 原始行号: 5
package server

import (
	"fmt"
	"net"
	"net/http"
	"strconv"

	"liuproxy_go/internal/agent"
	"liuproxy_go/internal/config"
)

// AppServer 代表整个应用服务器，可以是 local 或 remote
type AppServer struct {
	cfg *config.AppConfig
}

// New 创建一个新的 AppServer 实例
func New(cfg *config.AppConfig) *AppServer {
	return &AppServer{cfg: cfg}
}

// Run 启动服务器并根据模式（local/remote）开始监听
func (s *AppServer) Run() {
	if s.cfg.Mode == "local" {
		s.runLocal()
	} else if s.cfg.Mode == "remote" {
		s.runRemote()
	}
	// 阻塞主 goroutine，使服务持续运行
	select {}
}

// runLocal 启动 local 模式下的所有监听器
func (s *AppServer) runLocal() {
	fmt.Println("Local server starting......")
	go s.startListener(s.cfg.LocalConf.PortHttpFirst)
	go s.startListener(s.cfg.LocalConf.PortSocks5First)
	go s.startListener(s.cfg.LocalConf.PortHttpWsFirst)
	go s.startListener(s.cfg.LocalConf.PortHttpSecond)
	go s.startListener(s.cfg.LocalConf.PortSocks5Second)
	go s.startListener(s.cfg.LocalConf.PortHttpWsSecond)
}

// runRemote 启动 remote 模式下的所有监听器
func (s *AppServer) runRemote() {
	fmt.Println("Remote server starting......")
	go s.startListener(s.cfg.RemoteConf.PortHttpSvr)
	go s.startListener(s.cfg.RemoteConf.PortSocks5Svr)
	go s.startWsListener(s.cfg.RemoteConf.PortWsSvr)
}

// startListener 为基于 TCP 的代理启动一个通用监听器
func (s *AppServer) startListener(port int) {
	if port == 0 {
		return
	}
	addr := fmt.Sprintf("0.0.0.0:%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Printf("Error listening on %s: %v\n", addr, err)
		return
	}
	defer listener.Close()
	fmt.Println("启动服务器侦听：", listener.Addr().String())

	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}

		// 将连接交给分发器处理
		go s.dispatchConnection(conn)
	}
}

// startWsListener 专门为 remote 端的 WebSocket 服务启动 HTTP 监听器
func (s *AppServer) startWsListener(port int) {
	if port == 0 {
		return
	}

	agentCfg := agent.Config{
		IsServerSide: true,
		EncryptKey:   s.cfg.Crypt,
	}
	// WebSocketAgent 在 remote 端需要 bufferSize
	wsAgent := agent.NewWebSocketAgent(agentCfg, s.cfg.BufferSize)

	mux := http.NewServeMux()
	// 将 /ws 路径的请求交给 WebSocketAgent 的 HandleUpgrade 方法处理
	mux.HandleFunc("/ws", wsAgent.HandleUpgrade)

	addr := fmt.Sprintf("0.0.0.0:%d", port)
	fmt.Println("启动 WebSocket 服务器侦听：", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		fmt.Println("Error starting WebSocket server:", err)
	}
}

// dispatchConnection 根据端口号创建并运行正确的 agent
func (s *AppServer) dispatchConnection(conn net.Conn) {
	localAddr := conn.LocalAddr().(*net.TCPAddr)
	port := localAddr.Port
	var ag agent.Agent

	if s.cfg.Mode == "local" {
		ag = s.createLocalAgent(port, conn.RemoteAddr())
	} else { // remote
		ag = s.createRemoteAgent(port)
	}

	if ag != nil {
		ag.HandleConnection(conn)
	} else {
		conn.Close()
	}

	if ag != nil {
		// Agent 接口的 HandleConnection 方法是阻塞的，直到连接处理完毕
		ag.HandleConnection(conn)
	} else {
		conn.Close()
	}
}

// createLocalAgent 根据本地监听端口创建相应的客户端 agent
func (s *AppServer) createLocalAgent(port int, remoteAddr net.Addr) agent.Agent {
	var remoteConfig []string
	var remoteIndex int

	switch port {
	// --- 第一组：使用 remote_ip01 (索引 0) ---
	case s.cfg.LocalConf.PortHttpFirst, s.cfg.LocalConf.PortSocks5First, s.cfg.LocalConf.PortHttpWsFirst:
		remoteIndex = 0
		if len(s.cfg.LocalConf.RemoteIPs) > remoteIndex {
			remoteConfig = s.cfg.LocalConf.RemoteIPs[remoteIndex]
		}

	// --- 第二组：使用 remote_ip02 (索引 1) ---
	case s.cfg.LocalConf.PortHttpSecond, s.cfg.LocalConf.PortSocks5Second, s.cfg.LocalConf.PortHttpWsSecond:
		remoteIndex = 1
		if len(s.cfg.LocalConf.RemoteIPs) > remoteIndex {
			remoteConfig = s.cfg.LocalConf.RemoteIPs[remoteIndex]
		}
	default:
		return nil // 端口不匹配任何已知配置
	}

	if remoteConfig == nil {
		return nil
	}

	agentCfg := agent.Config{
		IsServerSide: false,
		EncryptKey:   s.cfg.Crypt,
		RemoteHost:   remoteConfig[0],
	}

	// 现在根据端口创建具体的 agent
	switch port {
	case s.cfg.LocalConf.PortHttpFirst, s.cfg.LocalConf.PortHttpSecond:
		if len(remoteConfig) < 2 {
			return nil
		}
		agentCfg.RemotePort = parseInt(remoteConfig[1])
		return agent.NewHTTPAgent(agentCfg, s.cfg.BufferSize)

	case s.cfg.LocalConf.PortSocks5First, s.cfg.LocalConf.PortSocks5Second:
		if len(remoteConfig) < 3 {
			return nil
		}
		agentCfg.RemotePort = parseInt(remoteConfig[2])
		return agent.NewSocks5Agent(agentCfg, s.cfg.BufferSize, remoteAddr)

	case s.cfg.LocalConf.PortHttpWsFirst, s.cfg.LocalConf.PortHttpWsSecond:
		if len(remoteConfig) < 5 {
			return nil
		}
		agentCfg.RemotePort = parseInt(remoteConfig[3])
		agentCfg.RemoteScheme = remoteConfig[4]
		return agent.NewWebSocketAgent(agentCfg, s.cfg.BufferSize)
	}

	return nil
}

// createRemoteAgent 根据远程监听端口创建相应的服务器端 agent
func (s *AppServer) createRemoteAgent(port int) agent.Agent {
	agentCfg := agent.Config{IsServerSide: true, EncryptKey: s.cfg.Crypt}
	switch port {
	case s.cfg.RemoteConf.PortHttpSvr:
		return agent.NewHTTPAgent(agentCfg, s.cfg.BufferSize)
	case s.cfg.RemoteConf.PortSocks5Svr:
		return agent.NewSocks5Agent(agentCfg, s.cfg.BufferSize, nil)
	}
	return nil
}

// parseInt 是一个辅助函数
func parseInt(s string) int {
	i, _ := strconv.Atoi(s)
	return i
}
