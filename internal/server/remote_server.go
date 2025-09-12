// --- START OF COMPLETE REPLACEMENT for remote_server.go ---
package server

import (
	"fmt"
	"log"
	"net"
	"net/http"

	"liuproxy_go/internal/agent/socks5"
	"liuproxy_go/internal/shared"
)

// runRemote 负责初始化和运行 remote 模式的所有服务
func (s *AppServer) runRemote() {
	log.Println("Initializing remote listeners...")

	// NewAgent 现在接收 *types.Config
	socksAgent := socks5.NewAgent(s.cfg, s.cfg.CommonConf.BufferSize).(*socks5.Agent)

	udpRelay, err := socks5.NewRemoteUDPRelay(*s.cfg, s.cfg.CommonConf.BufferSize)
	if err != nil {
		log.Fatalf("Failed to create Remote UDP Relay: %v", err)
	}
	socksAgent.SetUDPRelay(udpRelay)

	wsPort := s.cfg.RemoteConf.PortWsSvr
	if wsPort <= 0 {
		log.Fatalln("Remote WebSocket port (port_ws_svr) is not configured.")
		return
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/tunnel", func(w http.ResponseWriter, r *http.Request) {
		conn, err := shared.NewWebSocketConnAdapterServer(w, r)
		if err != nil {
			return
		}

		s.waitGroup.Add(1)
		go func() {
			defer s.waitGroup.Done()
			defer func() {
				if r := recover(); r != nil {
					log.Printf("!!!!!!!!!! FATAL: PANIC recovered in remote handler for %s !!!!!!!!!!", conn.RemoteAddr())
				}
				conn.Close()
			}()
			socksAgent.HandleConnection(conn, nil)
		}()
	})

	addr := fmt.Sprintf("0.0.0.0:%d", wsPort)
	log.Printf(">>> SUCCESS: Unified tunnel server listening on ws://%s/tunnel", addr)

	logLocalIPs(wsPort)

	// http.Server is more controllable than http.ListenAndServe
	httpServer := &http.Server{Addr: addr, Handler: mux}

	// Add the HTTP server's listener to our managed list so Stop() can close it.
	// Note: We don't have a direct listener object, so we manage the server itself.
	// For simplicity in this refactor, we'll run ListenAndServe in a goroutine
	// and rely on a more complex shutdown mechanism if needed later. For now,
	// this doesn't integrate with s.Stop() perfectly but is sufficient for cmd/remote.
	s.waitGroup.Add(1)
	go func() {
		defer s.waitGroup.Done()
		if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("ListenAndServe for WebSocket failed: %v", err)
		}
	}()
}

// logLocalIPs finds and prints available non-loopback IPv4 addresses.
func logLocalIPs(port int) {
	interfaces, err := net.Interfaces()
	if err != nil {
		log.Printf("Could not get network interfaces: %v", err)
		return
	}

	log.Println("--- Available server addresses for client configuration ---")
	for _, i := range interfaces {
		addrs, err := i.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			// We only care about IPv4 for simplicity
			ip = ip.To4()
			if ip != nil {
				log.Printf("  -> %s:%d", ip.String(), port)
			}
		}
	}
}
