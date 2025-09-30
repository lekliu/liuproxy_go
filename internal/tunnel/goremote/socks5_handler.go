// --- START OF COMPLETE REPLACEMENT for internal/goremote/socks5_handler.go ---
package goremote

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
)

func (a *Agent) handleSocks5(inboundConn net.Conn, reader *bufio.Reader) {
	cmd, targetAddr, err := a.HandshakeWithClient(inboundConn, reader)
	if err != nil {
		if err != io.EOF {
			log.Printf("[GoRemote SOCKS5] Handshake failed: %v", err)
		}
		return
	}
	switch cmd {
	case 1: // CONNECT
		session := a.sessionManager.NewTCPSession(inboundConn, targetAddr, nil, true)
		session.Wait()
	case 3: // UDP ASSOCIATE
		a.handleUdpAssociate(inboundConn)
	default:
		_, _ = inboundConn.Write([]byte{0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0}) // Command not supported
	}
}

func (a *Agent) handleUdpAssociate(clientTcpConn net.Conn) {
	if a.udpManager == nil {
		_, _ = clientTcpConn.Write([]byte{0x05, 0x01, 0x00, 0x01, 0, 0, 0, 0, 0, 0}) // General failure
		return
	}

	if a.GetCurrentBackendType() == "worker" {
		// --- 2/4 MODIFICATION END ---
		_, _ = clientTcpConn.Write([]byte{0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0}) // Command not supported
		return
	}

	session, err := a.udpManager.GetOrCreateSingletonSession()
	if err != nil {
		// --- 3/4 MODIFICATION END ---
		_, _ = clientTcpConn.Write([]byte{0x05, 0x01, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}

	localAddr := session.GetListenerAddr().(*net.UDPAddr)
	reply := []byte{0x05, 0x00, 0x00, 0x01}
	reply = append(reply, localAddr.IP.To4()...)
	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, uint16(localAddr.Port))
	reply = append(reply, portBytes...)

	if _, err := clientTcpConn.Write(reply); err != nil {
		return
	}
	io.Copy(io.Discard, clientTcpConn)
}

func (a *Agent) HandshakeWithClient(conn net.Conn, reader *bufio.Reader) (byte, string, error) {
	authBuf := make([]byte, 2)
	if _, err := io.ReadFull(reader, authBuf); err != nil {
		return 0, "", err
	}
	if authBuf[0] != 0x05 {
		return 0, "", fmt.Errorf("unsupported socks version: %d", authBuf[0])
	}
	nMethods := int(authBuf[1])
	if nMethods > 0 {
		if _, err := io.ReadFull(reader, make([]byte, nMethods)); err != nil {
			return 0, "", err
		}
	}
	if _, err := conn.Write([]byte{0x05, 0x00}); err != nil {
		return 0, "", err
	}
	reqHeader := make([]byte, 4)
	if _, err := io.ReadFull(reader, reqHeader); err != nil {
		return 0, "", err
	}
	cmd, addrType := reqHeader[1], reqHeader[3]
	var host string
	switch addrType {
	case 0x01: // IPv4
		addrBuf := make([]byte, 4)
		if _, err := io.ReadFull(reader, addrBuf); err != nil {
			return 0, "", err
		}
		host = net.IP(addrBuf).String()
	case 0x03: // Domain
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(reader, lenBuf); err != nil {
			return 0, "", err
		}
		addrBuf := make([]byte, lenBuf[0])
		if _, err := io.ReadFull(reader, addrBuf); err != nil {
			return 0, "", err
		}
		host = string(addrBuf)
	default:
		return 0, "", fmt.Errorf("unsupported address type: %d", addrType)
	}
	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(reader, portBuf); err != nil {
		return 0, "", err
	}
	port := binary.BigEndian.Uint16(portBuf)
	return cmd, net.JoinHostPort(host, strconv.Itoa(int(port))), nil
}

// --- END OF COMPLETE REPLACEMENT ---
