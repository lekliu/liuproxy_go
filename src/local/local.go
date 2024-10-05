package local

import (
	"fmt"
	"main/conf"
	"net"
)

const SOCKS_VERSION = 5

var (
	cfg *conf.AppConfig
)

type Server struct {
	listener net.Listener
}

func (s *Server) initSocket(host string, port int) {
	address := fmt.Sprintf("%s:%d", host, port)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		fmt.Println("Error initializing socket:", err)
		return
	}
	s.listener = listener
	fmt.Println("启动服务器侦听：", s.listener.Addr().String())
}

func (s *Server) start(host string, port int) {
	s.initSocket(host, port)
	var remoteID int
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			fmt.Println("Error accepting connection:", err)
			continue
		}

		accessHost := AccessToHost{}
		remoteID = 0
		if cfg.LocalConf.PortHttpSecond == port {
			remoteID = 1
		}
		go accessHost.Handler(conn, conn.RemoteAddr(), port, remoteID)
	}
}

func startServerThread(port int) {
	server := &Server{}
	server.start("0.0.0.0", port)
}

func RunServer(cfg1 *conf.AppConfig) {
	fmt.Println("Local server starting......")
	cfg = cfg1

	// Get local IP
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		fmt.Println("Error getting IP address:", err)
		return
	}
	localIP := conn.LocalAddr().(*net.UDPAddr).IP.String()
	conn.Close()
	fmt.Println(localIP)

	// Start server threads
	go startServerThread(cfg.LocalConf.PortHttpFirst)
	go startServerThread(cfg.LocalConf.PortHttpSecond)
	go startServerThread(cfg.LocalConf.PortSocks5)

	// 保持主线程运行
	select {}
}
