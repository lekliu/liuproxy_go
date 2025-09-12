// --- START OF COMPLETE REPLACEMENT for remote_handler.go ---
package socks5

import (
	"net"
)

func (a *Agent) handleRemote(inboundConn net.Conn) {
	// --- LOG ---
	tunnel := NewRemoteTunnel(inboundConn, a)
	tunnel.StartReadLoop()
}
