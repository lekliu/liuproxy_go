package local

import (
	"main/utils/string"
	"net"
)

// AccessToHost 结构体
type AccessToHost struct {
	conn net.Conn
	addr net.Addr
}

// Handler 方法
func (a *AccessToHost) Handler(conn net.Conn, addr net.Addr, port int) {
	a.conn = conn
	a.addr = addr

	if port == cfg.LocalConf.PortSocks5First {
		remoteIP := cfg.LocalConf.RemoteIPs[0][0]
		remotePort := string.StrToInt(cfg.LocalConf.RemoteIPs[0][2], 10090)
		// 启动 Socks5 代理
		go func() {
			socks5 := NewMySocks5(conn, addr, remoteIP, remotePort)
			socks5.Start()
		}()
		return
	}

	if port == cfg.LocalConf.PortSocks5Second {
		remoteIP := cfg.LocalConf.RemoteIPs[1][0]
		remotePort := string.StrToInt(cfg.LocalConf.RemoteIPs[1][2], 10090)
		// 启动 Socks5 代理
		go func() {
			socks5 := NewMySocks5(conn, addr, remoteIP, remotePort)
			socks5.Start()
		}()
		return
	}

	if port == cfg.LocalConf.PortHttpFirst {
		remoteIP := cfg.LocalConf.RemoteIPs[0][0]
		remotePort := string.StrToInt(cfg.LocalConf.RemoteIPs[0][1], 10089)
		// 启动 Socks5 代理
		go func() {
			proxy := NewMyProxy(conn, remoteIP, remotePort)
			proxy.Start()
		}()
		return
	}

	if port == cfg.LocalConf.PortHttpSecond {
		remoteIP := cfg.LocalConf.RemoteIPs[1][0]
		remotePort := string.StrToInt(cfg.LocalConf.RemoteIPs[1][1], 10089)
		// 启动 Socks5 代理
		go func() {
			proxy := NewMyProxy(conn, remoteIP, remotePort)
			proxy.Start()
		}()
		return
	}
}
