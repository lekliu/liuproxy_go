package gateway

import (
	"encoding/binary"
	"fmt"
	"io"
	"liuproxy_go/internal/shared/types"
	"net"
	"strconv"
	"time"
)

// dialSocksProxy 函数将作为 SOCKS5 客户端连接到后端代理，并请求连接到最终目标。
func dialSocksProxy(backendAddr, targetAddr, serverID string, failureReporter types.FailureReporter) (net.Conn, error) {
	// 1. 连接到后端 SOCKS5 代理服务器
	conn, err := net.DialTimeout("tcp", backendAddr, 10*time.Second)
	if err != nil {
		if failureReporter != nil {
			// logger.Debug() is now removed as it's part of the caller's responsibility
			failureReporter.ReportFailure(serverID)
		}
		return nil, fmt.Errorf("socks_client: failed to connect to backend proxy '%s': %w", backendAddr, err)
	}

	// 2. 发送认证请求 (无认证)
	// VER=5, NMETHODS=1, METHODS=0x00(No Auth)
	authRequest := []byte{0x05, 0x01, 0x00}
	if _, err := conn.Write(authRequest); err != nil {
		conn.Close()
		return nil, fmt.Errorf("socks_client: failed to send auth request: %w", err)
	}

	// 3. 读取并验证认证响应
	authResponse := make([]byte, 2)
	if _, err := io.ReadFull(conn, authResponse); err != nil {
		conn.Close()
		return nil, fmt.Errorf("socks_client: failed to read auth response: %w", err)
	}
	if authResponse[0] != 0x05 || authResponse[1] != 0x00 {
		conn.Close()
		return nil, fmt.Errorf("socks_client: backend proxy requires authentication or returned invalid response: %v", authResponse)
	}

	// 4. 发送 CONNECT 请求
	host, portStr, err := net.SplitHostPort(targetAddr)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("socks_client: invalid target address '%s': %w", targetAddr, err)
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("socks_client: invalid target port '%s': %w", portStr, err)
	}

	// 构建请求包: VER, CMD, RSV, ATYP, DST.ADDR, DST.PORT
	req := []byte{0x05, 0x01, 0x00} // VER=5, CMD=1(CONNECT), RSV=0

	if ip := net.ParseIP(host); ip != nil {
		if ipv4 := ip.To4(); ipv4 != nil {
			req = append(req, 0x01) // ATYP=1(IPv4)
			req = append(req, ipv4...)
		} else {
			req = append(req, 0x04) // ATYP=4(IPv6)
			req = append(req, ip.To16()...)
		}
	} else {
		if len(host) > 255 {
			conn.Close()
			return nil, fmt.Errorf("socks_client: hostname too long: %s", host)
		}
		req = append(req, 0x03) // ATYP=3(Domain)
		req = append(req, byte(len(host)))
		req = append(req, host...)
	}

	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, uint16(port))
	req = append(req, portBytes...)

	if _, err := conn.Write(req); err != nil {
		conn.Close()
		return nil, fmt.Errorf("socks_client: failed to send connect request: %w", err)
	}

	// 5. 读取并验证最终响应
	resp := make([]byte, 10) // 通常 IPv4 响应是 10 字节
	if _, err := io.ReadFull(conn, resp[:4]); err != nil {
		conn.Close()
		return nil, fmt.Errorf("socks_client: failed to read final response header: %w", err)
	}

	if resp[0] != 0x05 {
		conn.Close()
		return nil, fmt.Errorf("socks_client: invalid response version: %d", resp[0])
	}
	if resp[1] != 0x00 {
		conn.Close()
		return nil, fmt.Errorf("socks_client: connection failed with status: %d", resp[1])
	}

	// 根据 ATYP 读取剩余的 BND.ADDR 和 BND.PORT
	addrType := resp[3]
	addrLen := 0
	switch addrType {
	case 0x01: // IPv4
		addrLen = 4
	case 0x04: // IPv6
		addrLen = 16
	case 0x03: // Domain
		// 第一个字节是长度
		lenByte := make([]byte, 1)
		if _, err := io.ReadFull(conn, lenByte); err != nil {
			conn.Close()
			return nil, fmt.Errorf("socks_client: failed to read domain len in response: %w", err)
		}
		addrLen = int(lenByte[0])
	default:
		conn.Close()
		return nil, fmt.Errorf("socks_client: unknown address type in response: %d", addrType)
	}

	// 读取地址和端口
	remainingLen := addrLen + 2 // address + port
	if _, err := io.ReadFull(conn, make([]byte, remainingLen)); err != nil {
		conn.Close()
		return nil, fmt.Errorf("socks_client: failed to read remaining response data: %w", err)
	}

	// 握手成功，返回可用于数据传输的连接
	return conn, nil
}
