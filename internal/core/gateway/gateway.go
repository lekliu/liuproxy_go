package gateway

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"io"
	"liuproxy_go/internal/shared/logger"
	"liuproxy_go/internal/shared/types"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Protocol string

const (
	ProtoSOCKS5  Protocol = "SOCKS5"
	ProtoHTTP    Protocol = "HTTP"
	ProtoTLS     Protocol = "TLS"
	ProtoUnknown Protocol = "UNKNOWN"
)

type Gateway struct {
	listener        net.Listener
	dispatcher      types.Dispatcher
	failureReporter types.FailureReporter
	closeOnce       sync.Once
	waitGroup       sync.WaitGroup
	listenPort      int
	directConn      VirtualStrategy
	rejectConn      VirtualStrategy
}

func New(listenPort int, dispatcher types.Dispatcher, failureReporter types.FailureReporter) *Gateway {
	return &Gateway{
		listenPort:      listenPort,
		dispatcher:      dispatcher,
		failureReporter: failureReporter,
		directConn:      NewDirectStrategy(),
		rejectConn:      NewRejectStrategy(),
	}
}

func (g *Gateway) Start() error {
	listenAddr := fmt.Sprintf("0.0.0.0:%d", g.listenPort)
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("gateway failed to listen on %s: %w", listenAddr, err)
	}
	g.listener = listener
	logger.Info().Str("listen_addr", listener.Addr().String()).Msg(">>> Gateway is listening on unified port.")

	g.waitGroup.Add(1)
	go g.acceptLoop()
	return nil
}

func (g *Gateway) acceptLoop() {
	defer g.waitGroup.Done()
	for {
		conn, err := g.listener.Accept()
		if err != nil {
			if opErr, ok := err.(*net.OpError); ok && strings.Contains(opErr.Err.Error(), "use of closed network connection") {
				logger.Info().Msg("Gateway listener is closing.")
				return
			}
			logger.Warn().Err(err).Msg("Gateway failed to accept connection")
			continue
		}
		g.waitGroup.Add(1)
		go g.handleConnection(conn)
	}
}

func (g *Gateway) handleConnection(inboundConn net.Conn) {
	defer g.waitGroup.Done()
	defer inboundConn.Close()

	// 1. 生成 Trace ID 并创建带上下文的 logger
	traceID := uuid.NewString()
	l := log.With().Str("trace_id", traceID).Logger()
	ctx := l.WithContext(context.Background())
	clientIP := inboundConn.RemoteAddr().String()
	inboundReader := bufio.NewReader(inboundConn)

	// 2. 嗅探目标和协议
	targetDest, proto, _, err := sniffTargetForRouting(inboundConn, inboundReader)
	if err != nil {
		l.Warn().Err(err).Str("client_ip", clientIP).Msg("Could not determine target")
		return
	}
	l.Debug().Str("proto", string(proto)).Str("client_ip", clientIP).Str("target", targetDest).Msg("Gateway: Sniffed target for routing")

	// 3. Dispatcher 获取后端地址，传递 context
	backendAddr, serverID, err := g.dispatcher.Dispatch(ctx, inboundConn.RemoteAddr(), targetDest)
	if err != nil {
		l.Warn().Err(err).Str("client_ip", clientIP).Str("target", targetDest).Msg("Gateway: Dispatcher returned error")
		return
	}

	// 4. 直接策略或拒绝策略
	switch backendAddr {
	case "DIRECT":
		targetNetAddr, _ := net.ResolveTCPAddr("tcp", targetDest)
		g.directConn.Handle(inboundConn, inboundReader, targetNetAddr)
		return
	case "REJECT":
		targetNetAddr, _ := net.ResolveTCPAddr("tcp", targetDest)
		g.rejectConn.Handle(inboundConn, inboundReader, targetNetAddr)
		return
	}

	// 5. 根据协议透传
	switch proto {
	case ProtoSOCKS5:
		g.forwardSocks5(inboundConn, inboundReader, targetDest, backendAddr, serverID)
	case ProtoHTTP:
		g.handleHttpProxy(ctx, inboundConn, inboundReader, targetDest, backendAddr, serverID)
	case ProtoTLS:
		g.forwardTCP(inboundConn, inboundReader, targetDest, backendAddr, serverID)
	default:
		l.Warn().Str("client_ip", clientIP).Msg("Unsupported protocol")
	}
}

func sniffTargetForRouting(conn net.Conn, reader *bufio.Reader) (target string, ptl Protocol, req *http.Request, err error) {
	// 确保至少有一个字节可供嗅探
	if err := fillBuffer(conn, reader, 1); err != nil {
		return "", ProtoUnknown, nil, fmt.Errorf("failed to read initial byte: %w", err)
	}
	firstByte, _ := reader.Peek(1)

	switch {
	case firstByte[0] == 0x05: // SOCKS5
		target, err := sniffTargetSocks5(conn, reader)
		return target, ProtoSOCKS5, nil, err
	case firstByte[0] == 0x16: // TLS ClientHello
		host, tlsErr := sniffTargetTLS(conn, reader)
		if tlsErr == nil && host != "" {
			return host, ProtoTLS, nil, nil
		}
		return "", ProtoUnknown, nil, fmt.Errorf("TLS SNI sniff failed: %w", tlsErr)
	case firstByte[0] >= 'A' && firstByte[0] <= 'Z': // HTTP Methods (GET, POST, CONNECT, etc.)
		host, request, httpErr := sniffTargetHTTP(conn, reader)
		if httpErr == nil && host != "" {
			return host, ProtoHTTP, request, nil
		}
		return "", ProtoUnknown, nil, fmt.Errorf("HTTP sniff failed: %w", httpErr)
	default:
		return "", ProtoUnknown, nil, fmt.Errorf("could not determine target protocol, initial byte: 0x%02x", firstByte[0])
	}
}

// sniffTargetTLS 被动嗅探 TLS ClientHello 中的 SNI (Server Name Indication)
func sniffTargetTLS(conn net.Conn, reader *bufio.Reader) (string, error) {
	// 确保缓冲区至少有5个字节 (TLS Record Header)
	if err := fillBuffer(conn, reader, 5); err != nil {
		return "", err
	}
	header, _ := reader.Peek(5)

	if header[0] != 0x16 {
		return "", fmt.Errorf("not a TLS handshake record")
	}

	if header[1] != 0x03 {
		return "", fmt.Errorf("unexpected TLS major version: %d", header[1])
	}

	recordLen := int(binary.BigEndian.Uint16(header[3:5]))
	totalHelloLen := 5 + recordLen

	if err := fillBuffer(conn, reader, totalHelloLen); err != nil {
		return "", fmt.Errorf("buffer does not contain full TLS ClientHello")
	}

	data, _ := reader.Peek(totalHelloLen)
	data = data[5:]

	if len(data) < 42 {
		return "", fmt.Errorf("invalid ClientHello: too short")
	}

	if data[0] != 0x01 {
		return "", fmt.Errorf("not a ClientHello message")
	}

	offset := 38

	sessionIDLen := int(data[offset])
	offset += 1 + sessionIDLen
	if offset+2 > len(data) {
		return "", fmt.Errorf("invalid ClientHello: session ID parsing error")
	}

	cipherSuitesLen := int(binary.BigEndian.Uint16(data[offset : offset+2]))
	offset += 2 + cipherSuitesLen
	if offset+1 > len(data) {
		return "", fmt.Errorf("invalid ClientHello: cipher suites parsing error")
	}

	compressionMethodsLen := int(data[offset])
	offset += 1 + compressionMethodsLen
	if offset+2 > len(data) {
		return "", fmt.Errorf("no extensions found")
	}

	extensionsLen := int(binary.BigEndian.Uint16(data[offset : offset+2]))
	offset += 2
	if offset+extensionsLen > len(data) {
		return "", fmt.Errorf("invalid ClientHello: extensions length mismatch")
	}
	extensionsData := data[offset : offset+extensionsLen]

	for len(extensionsData) >= 4 {
		extType := binary.BigEndian.Uint16(extensionsData[0:2])
		extLen := int(binary.BigEndian.Uint16(extensionsData[2:4]))
		extensionsData = extensionsData[4:]

		if len(extensionsData) < extLen {
			return "", fmt.Errorf("invalid extension length")
		}

		if extType == 0x0000 { // SNI
			sniData := extensionsData[:extLen]
			if len(sniData) < 5 {
				return "", fmt.Errorf("invalid SNI data")
			}
			sniData = sniData[2:]
			if sniData[0] != 0x00 {
				return "", fmt.Errorf("unsupported SNI name type: %d", sniData[0])
			}
			nameLen := int(binary.BigEndian.Uint16(sniData[1:3]))
			sniData = sniData[3:]
			if len(sniData) < nameLen {
				return "", fmt.Errorf("invalid SNI name length")
			}
			return string(sniData[:nameLen]), nil
		}
		extensionsData = extensionsData[extLen:]
	}

	return "", fmt.Errorf("SNI not found")
}

// forwardTCP 是一个通用的 L4 TCP 转发器
func (g *Gateway) forwardTCP(inboundConn net.Conn, inboundReader *bufio.Reader, target string, backendAddr string, serverID string) {
	outboundConn, err := net.Dial("tcp", backendAddr)
	if err != nil {
		logger.Error().Err(err).Str("backend_addr", backendAddr).Msg("Gateway: Failed to dial backend for TLS")
		if g.failureReporter != nil {
			g.failureReporter.ReportFailure(serverID)
		}
		return
	}

	if g.failureReporter != nil {
		g.failureReporter.ReportSuccess(serverID)
	}
	defer outboundConn.Close()

	var wg sync.WaitGroup
	wg.Add(2)
	clientAddr := inboundConn.RemoteAddr().String()
	remoteAddr := outboundConn.RemoteAddr().String()

	go func() {
		defer wg.Done()
		bytesCopied, err := io.Copy(outboundConn, inboundReader)
		logger.Info().Str("trace", "PIPE-FORWARD-TCP").Str("direction", "Client -> Backend").
			Str("client_addr", clientAddr).Str("backend_addr", remoteAddr).Int64("bytes", bytesCopied).Err(err).Msg("Pipe finished")
		if tcp, ok := outboundConn.(*net.TCPConn); ok {
			tcp.CloseWrite()
		}
	}()

	go func() {
		defer wg.Done()
		bytesCopied, err := io.Copy(inboundConn, outboundConn)
		logger.Info().Str("trace", "PIPE-FORWARD-TCP").Str("direction", "Backend -> Client").
			Str("client_addr", clientAddr).Str("backend_addr", remoteAddr).Int64("bytes", bytesCopied).Err(err).Msg("Pipe finished")
		if tcp, ok := inboundConn.(*net.TCPConn); ok {
			tcp.CloseWrite()
		}
	}()
	wg.Wait()
}

func (g *Gateway) Close() {
	g.closeOnce.Do(func() {
		if g.listener != nil {
			g.listener.Close()
		}
		g.waitGroup.Wait()
		log.Info().Msg("Gateway has been shut down")
	})
}

func fillBuffer(conn net.Conn, reader *bufio.Reader, n int) error {
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	defer conn.SetReadDeadline(time.Time{})
	for reader.Buffered() < n {
		_, err := reader.Peek(1)
		if err != nil {
			return err
		}
	}
	return nil
}

// handleHttpProxy 实现了完整的 HTTP/HTTPS 代理逻辑。
// 它将 HTTP 请求转换为后端的 SOCKS5 请求。
func (g *Gateway) handleHttpProxy(ctx context.Context, inboundConn net.Conn, inboundReader *bufio.Reader, targetDest, backendAddr, serverID string) {
	logger.Debug().Str("targetDest:", targetDest).Msg("handleHttpProxy => ")
	clientIP := inboundConn.RemoteAddr().String()

	bufferedBytes, _ := inboundReader.Peek(inboundReader.Buffered())
	req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(bufferedBytes)))
	if err != nil {
		logger.Error().Err(err).Str("client_ip", clientIP).Msg("Gateway: Failed to re-parse HTTP request for proxy logic.")
		inboundConn.Close()
		return
	}

	backendConn, err := dialSocksProxy(backendAddr, targetDest, serverID, g.failureReporter)
	if err != nil {
		log.Ctx(ctx).Error().Err(err).
			Str("client_ip", clientIP).
			Str("target", targetDest).
			Str("backend", backendAddr).
			Str("server_id", serverID).
			Msg("Gateway: Failed to establish SOCKS tunnel. Reporting failure.")

		inboundConn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		inboundConn.Close()
		return
	}
	defer backendConn.Close()

	if req.Method == "CONNECT" {
		b := make([]byte, inboundReader.Buffered())
		inboundReader.Read(b)
		_, err = inboundConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
		if err != nil {
			logger.Error().Err(err).Str("client_ip", clientIP).Msg("Gateway: Failed to send CONNECT OK response to client.")
			return
		}
	} else {
		bytesToWrite := make([]byte, inboundReader.Buffered())
		n, _ := inboundReader.Read(bytesToWrite)
		if _, err := backendConn.Write(bytesToWrite[:n]); err != nil {
			logger.Error().Err(err).
				Str("client_ip", clientIP).
				Str("target", targetDest).
				Msg("Gateway: Failed to forward initial HTTP request through tunnel.")
			return
		}
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		io.Copy(backendConn, inboundConn)
		if tcpConn, ok := backendConn.(interface{ CloseWrite() error }); ok {
			tcpConn.CloseWrite()
		}
	}()

	go func() {
		defer wg.Done()
		io.Copy(inboundConn, backendConn)
		if tcpConn, ok := inboundConn.(interface{ CloseWrite() error }); ok {
			tcpConn.CloseWrite()
		}
	}()

	wg.Wait()
	logger.Debug().Str("client_ip", clientIP).Str("target", targetDest).Msg("Gateway: HTTP proxy session finished.")
}
