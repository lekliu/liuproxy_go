package remote

import (
	"net"
)

type AccessHost struct {
	conn net.Conn
	addr net.Addr
}

func (a *AccessHost) handler(conn net.Conn, addr net.Addr, myPort int) {
	a.conn = conn
	a.addr = addr

	if myPort == cfg.RemoteConf.PortSocks5Svr {
		socks5 := NewMySocks5(a.conn)
		socks5.Start()
		return
	}

	if myPort == cfg.RemoteConf.PortHttpSvr {
		host := NewMyHost(a.conn)
		host.Start()
		return
	}
}
