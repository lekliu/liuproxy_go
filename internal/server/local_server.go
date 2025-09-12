// --- START OF COMPLETE REPLACEMENT for local_server.go ---
package server

import (
	"bufio"
	"fmt"
	"liuproxy_go/internal/agent/socks5"
	"log"
	"net"
	"time"
)

// runLocal 负责初始化和运行 local 模式的所有服务
func (s *AppServer) runLocal() {
	log.Println("Initializing local listeners...")

	if len(s.cfg.LocalConf.RemoteIPs) == 0 {
		err := fmt.Errorf("no remote server configured in local.ini (e.g., remote_ip_01)")
		if s.isMobile {
			s.mobileErrChan <- err
		}
		return
	}

	// 1. 将配置文件中的字符串数组转换为 BackendProfile 结构体数组
	profiles := parseProfilesFromConfig(s.cfg.LocalConf.RemoteIPs)

	// 2. 使用策略工厂创建当前激活的策略实例
	strategy, err := socks5.CreateStrategyFromConfig(s.cfg, profiles)
	if err != nil {
		if s.isMobile {
			s.mobileErrChan <- fmt.Errorf("failed to create tunnel strategy: %w", err)
		} else {
			log.Fatalf("Failed to create tunnel strategy: %v", err)
		}
		return
	}
	s.strategy = strategy
	log.Printf(">>> Activated strategy: '%s'", s.strategy.GetType())

	// 3. 初始化策略 (对于Go Remote, 这会触发预连接)
	// For mobile, we need to block and see if initialization (pre-connection) succeeds
	if s.isMobile {
		// For mobile, we block and wait for initialization (pre-connection).
		// We can call it directly now without a channel because RunMobile is already in a goroutine.
		err := strategy.Initialize()
		if err != nil {
			s.mobileErrChan <- fmt.Errorf("strategy initialization failed: %w", err)
			return
		}
	}

	// 4. 启动统一端口的监听器，并将所有连接交给 dispatchConnection 处理
	s.startListener(s.cfg.LocalConf.UnifiedPort, "unified", s.dispatchConnection)
}

func (s *AppServer) startListener(port int, portType string, handler func(net.Conn)) {
	addr := fmt.Sprintf("0.0.0.0:%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		if s.isMobile {
			s.mobileErrChan <- fmt.Errorf("failed to listen on %s: %w", addr, err)
		}
		log.Printf("!!! FAILED to listen on %s: %v", addr, err)
		return
	}
	log.Printf(">>> SUCCESS: Listening for '%s' connections on %s", portType, listener.Addr().String())

	s.listeners = append(s.listeners, &ManagedListener{Listener: listener, PortType: portType})
	if s.isMobile {
		s.mobileListenerChan <- listener
	}

	s.waitGroup.Add(1)
	go func() {
		defer s.waitGroup.Done()
		defer listener.Close()
		for {
			conn, err := listener.Accept()
			if err != nil {
				log.Printf("Listener for '%s' on %s stopped.", portType, listener.Addr().String())
				return
			}
			s.waitGroup.Add(1)
			go func(c net.Conn) {
				defer s.waitGroup.Done()
				defer func() {
					if r := recover(); r != nil {
						log.Printf("Panic recovered in connection handler: %v", r)
					}
					c.Close()
				}()
				handler(c)
			}(conn)
		}
	}()
}

// dispatchConnection 现在将所有连接处理工作委托给当前激活的策略
func (s *AppServer) dispatchConnection(conn net.Conn) {
	if err := conn.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return
	}

	reader := bufio.NewReader(conn)
	// 预读一个字节用于后续可能的协议嗅探，但把嗅探的责任交给了策略自己
	_, err := reader.Peek(1)
	if err != nil {
		return
	}

	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		return
	}

	s.strategy.HandleConnection(conn, reader)
}

// --- END OF COMPLETE REPLACEMENT ---
