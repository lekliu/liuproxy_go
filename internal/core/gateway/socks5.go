package gateway

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"liuproxy_go/internal/shared/logger"
	"net"
	"strconv"
	"sync"
)

// 嗅探目标
func sniffTargetSocks5(conn net.Conn, reader *bufio.Reader) (string, error) {
	err := handleSocks5ClientHandshake(conn, reader)
	if err != nil {
		return "handleSocks5ClientHandshake ", err
	}

	reqHeaderSize := 4
	if err := fillBuffer(conn, reader, reqHeaderSize); err != nil {
		return "", err
	}
	reqHeader, _ := reader.Peek(reqHeaderSize)
	if reqHeader[0] != 0x05 || reqHeader[1] != 0x01 {
		return "", fmt.Errorf("not a SOCKS5 CONNECT request")
	}
	addrType := reqHeader[3]
	addrBodyOffset := reqHeaderSize
	var host string
	var port int
	switch addrType {
	case 0x01: // IPv4
		peekSize := addrBodyOffset + 4 + 2
		if err := fillBuffer(conn, reader, peekSize); err != nil {
			return "", err
		}
		fullHeader, _ := reader.Peek(peekSize)
		host = net.IP(fullHeader[addrBodyOffset : addrBodyOffset+4]).String()
		port = int(binary.BigEndian.Uint16(fullHeader[addrBodyOffset+4 : addrBodyOffset+6]))
	case 0x03: // Domain
		peekSize := addrBodyOffset + 1
		if err := fillBuffer(conn, reader, peekSize); err != nil {
			return "", err
		}
		lenHeader, _ := reader.Peek(peekSize)
		domainLen := int(lenHeader[addrBodyOffset])
		peekSize = addrBodyOffset + 1 + domainLen + 2
		if err := fillBuffer(conn, reader, peekSize); err != nil {
			return "", err
		}
		fullHeader, _ := reader.Peek(peekSize)
		host = string(fullHeader[addrBodyOffset+1 : addrBodyOffset+1+domainLen])
		port = int(binary.BigEndian.Uint16(fullHeader[addrBodyOffset+1+domainLen : addrBodyOffset+1+domainLen+2]))
	case 0x04: // IPv6
		peekSize := addrBodyOffset + 16 + 2
		if err := fillBuffer(conn, reader, peekSize); err != nil {
			return "", err
		}
		fullHeader, _ := reader.Peek(peekSize)
		host = net.IP(fullHeader[addrBodyOffset : addrBodyOffset+16]).String()
		port = int(binary.BigEndian.Uint16(fullHeader[addrBodyOffset+16 : addrBodyOffset+18]))
	default:
		return "", fmt.Errorf("unsupported SOCKS5 address type: %d", addrType)
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}

func handleSocks5ClientHandshake(conn net.Conn, reader *bufio.Reader) error {
	header := make([]byte, 2)
	if _, err := io.ReadFull(reader, header); err != nil {
		return err
	}
	nMethods := int(header[1])
	methods := make([]byte, nMethods)
	if _, err := io.ReadFull(reader, methods); err != nil {
		return err
	}
	_, err := conn.Write([]byte{0x05, 0x00})
	return err
}

func handleSocks5BackendHandshake(backendConn net.Conn, backendReader *bufio.Reader) error {
	_, err := backendConn.Write([]byte{0x05, 0x01, 0x00})
	if err != nil {
		return err
	}
	resp := make([]byte, 2)
	if _, err := io.ReadFull(backendReader, resp); err != nil {
		return err
	}
	if resp[0] != 0x05 {
		return fmt.Errorf("not a SOCKS5 CONNECT request")
	}
	return nil
}

func (g *Gateway) forwardSocks5(inboundConn net.Conn, inboundReader *bufio.Reader, target string, backendAddr string, serverID string) {
	outboundConn, err := net.Dial("tcp", backendAddr)
	if err != nil {
		logger.Error().Err(err).Str("backend_addr", backendAddr).Msg("SOCKS5: Failed to dial backend")
		if g.failureReporter != nil {
			g.failureReporter.ReportFailure(serverID)
		}
		return
	}
	if g.failureReporter != nil {
		g.failureReporter.ReportSuccess(serverID)
	}
	defer outboundConn.Close()
	outboundReader := bufio.NewReader(outboundConn)
	if err := handleSocks5BackendHandshake(outboundConn, outboundReader); err != nil {
		logger.Error().Err(err).Str("backend_addr", backendAddr).Msg("SOCKS5: backend handshake failed")
		return
	}
	var wg sync.WaitGroup
	wg.Add(2)
	clientAddr := inboundConn.RemoteAddr().String()
	remoteAddr := outboundConn.RemoteAddr().String()

	go func() {
		defer wg.Done()
		bytesCopied, err := io.Copy(outboundConn, inboundReader)
		logger.Info().Str("trace", "PIPE-FORWARD-SOCKS5").Str("direction", "Client -> Backend").
			Str("client_addr", clientAddr).Str("backend_addr", remoteAddr).Int64("bytes", bytesCopied).Err(err).Msg("Pipe finished")
		if tcp, ok := outboundConn.(*net.TCPConn); ok {
			tcp.CloseWrite()
		}
	}()
	go func() {
		defer wg.Done()
		bytesCopied, err := io.Copy(inboundConn, outboundReader)
		logger.Info().Str("trace", "PIPE-FORWARD-SOCKS5").Str("direction", "Backend -> Client").
			Str("client_addr", clientAddr).Str("backend_addr", remoteAddr).Int64("bytes", bytesCopied).Err(err).Msg("Pipe finished")
		if tcp, ok := inboundConn.(*net.TCPConn); ok {
			tcp.CloseWrite()
		}
	}()
	wg.Wait()
}
