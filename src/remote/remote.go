package remote

import (
	"fmt"
	"main/conf"
	"net"
	"sync"
)

const SOCKS_VERSION = 5

var (
	cfg *conf.AppConfig
)

// Server struct
type Server struct {
	listener net.Listener
	mu       sync.Mutex
}

func (server *Server) initSocket(host string, port int) error {
	addr := fmt.Sprintf("%s:%d", host, port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	server.listener = ln
	fmt.Println("启动服务器侦听：", ln.Addr().String())
	return nil
}

func (server *Server) start(host string, port int) {
	err := server.initSocket(host, port)
	if err != nil {
		fmt.Println("Error initializing socket:", err)
		return
	}
	defer server.listener.Close()

	for {
		conn, err := server.listener.Accept()
		if err != nil {
			fmt.Println("Error accepting connection:", err)
			continue
		}
		go func() {
			accessHost := &ACCESS_HOST{}
			// AccessHost struct and handler method
			accessHost.handler(conn, conn.RemoteAddr(), port)
		}()
	}
}

func startServerThread(port int) {
	host := "0.0.0.0"
	server := &Server{}
	go server.start(host, port)
}

func RunServer(cfg1 *conf.AppConfig) {
	fmt.Println("Remote server starting......")
	cfg = cfg1

	// 获取本机IP
	var ip string
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err == nil {
		defer conn.Close()
		localAddr := conn.LocalAddr().(*net.UDPAddr)
		ip = localAddr.IP.String()
	}
	fmt.Println(ip)

	startServerThread(cfg.RemoteConf.PortHttpSvr)
	startServerThread(cfg.RemoteConf.PortSocks5Svr)
	startServerThread(cfg.RemoteConf.PortSocks8Svr)

	// 保持主线程运行
	select {}
}
