// --- START OF COMPLETE REPLACEMENT for local_handler.go ---
package socks5

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
)

func (a *Agent) handleLocal(inboundConn net.Conn, reader *bufio.Reader) {
	cmd, targetAddr, err := a.handshakeWithClient(inboundConn, reader)
	if err != nil {
		if err != io.EOF && err != io.ErrUnexpectedEOF {
			log.Printf("[SOCKS5_LOCAL] Handshake with client %s failed: %v", inboundConn.RemoteAddr(), err)
		}
		return
	}
	switch cmd {
	case 1:
		a.handleLocalTCP(inboundConn, targetAddr)
	case 3:
		a.handleLocalUdpAssociate(inboundConn)
	default:
		_, _ = inboundConn.Write([]byte{0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
	}
}
func (a *Agent) handleLocalTCP(inboundConn net.Conn, targetAddr string) {
	if a.tunnelManager == nil {
		log.Printf("[LOCAL_TCP] FATAL: TunnelManager is not initialized for client %s!", inboundConn.RemoteAddr())
		return
	}
	// 对于 SOCKS5，没有初始数据，isSSL 默认为 true (CONNECT 命令)
	session := a.tunnelManager.SessionManager.NewTCPSession(inboundConn, targetAddr, nil, true)
	session.Wait()
}
func (a *Agent) handleLocalUdpAssociate(clientTcpConn net.Conn) {
	if a.tunnelManager != nil && a.tunnelManager.GetCurrentBackendType() == "worker" {
		// SOCKS5 "Command not supported" 响应
		_, _ = clientTcpConn.Write([]byte{0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return // 直接返回，不再继续处理
	}

	if a.udpManager == nil {
		_, _ = clientTcpConn.Write([]byte{0x05, 0x01, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}

	session, err := a.udpManager.GetOrCreateSingletonSession()
	if err != nil {
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
func (a *Agent) handshakeWithClient(conn net.Conn, reader *bufio.Reader) (byte, string, error) {
	authBuf := make([]byte, 257)
	if _, err := io.ReadFull(reader, authBuf[:2]); err != nil {
		return 0, "", fmt.Errorf("reading auth header failed: %w", err)
	}

	if authBuf[0] != 0x05 {
		return 0, "", fmt.Errorf("unsupported socks version: %d", authBuf[0])
	}
	nMethods := int(authBuf[1])
	if nMethods > 0 {
		methodsSlice := authBuf[:nMethods]
		if _, err := io.ReadFull(reader, methodsSlice); err != nil {
			return 0, "", fmt.Errorf("reading auth methods failed: %w", err)
		}
	}

	authResponse := []byte{0x05, 0x00}
	if _, err := conn.Write(authResponse); err != nil {
		return 0, "", fmt.Errorf("writing auth response failed: %w", err)
	}

	reqHeader := make([]byte, 4)
	if _, err := io.ReadFull(reader, reqHeader); err != nil {
		return 0, "", fmt.Errorf("reading request header failed: %w", err)
	}

	if reqHeader[0] != 0x05 {
		return 0, "", fmt.Errorf("invalid version in request: %d", reqHeader[0])
	}
	cmd := reqHeader[1]
	addrType := reqHeader[3]
	var host, portStr string
	switch addrType {
	case 0x01:
		addrBuf := make([]byte, 4)
		if _, err := io.ReadFull(reader, addrBuf); err != nil {
			return 0, "", err
		}
		host = net.IP(addrBuf).String()
	case 0x03:
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(reader, lenBuf); err != nil {
			return 0, "", err
		}
		domainLen := int(lenBuf[0])
		addrBuf := make([]byte, domainLen)
		if _, err := io.ReadFull(reader, addrBuf); err != nil {
			return 0, "", err
		}
		host = string(addrBuf)
	case 0x04:
		addrBuf := make([]byte, 16)
		if _, err := io.ReadFull(reader, addrBuf); err != nil {
			return 0, "", err
		}
		host = net.IP(addrBuf).String()
	default:
		return 0, "", fmt.Errorf("unsupported address type: %d", addrType)
	}
	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(reader, portBuf); err != nil {
		return 0, "", fmt.Errorf("reading port failed: %w", err)
	}
	portStr = strconv.Itoa(int(binary.BigEndian.Uint16(portBuf)))
	targetAddress := net.JoinHostPort(host, portStr)
	return cmd, targetAddress, nil
}
