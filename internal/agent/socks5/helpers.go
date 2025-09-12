// --- START OF NEW FILE internal/agent/socks5/helpers.go ---
package socks5

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"strings"
)

// buildMetadata 根据协议 V2 构建元数据包
func (a *Agent) buildMetadata(cmd byte, targetAddr string) []byte {
	host, portStr, _ := net.SplitHostPort(targetAddr)
	port, _ := strconv.Atoi(portStr)
	addrBytes := []byte(host)
	addrType := byte(0x03)
	if ip := net.ParseIP(host); ip != nil {
		if ipv4 := ip.To4(); ipv4 != nil {
			addrType = 0x01
			addrBytes = ipv4
		} else {
			addrType = 0x04
			addrBytes = ip.To16()
		}
	}
	var buf bytes.Buffer
	buf.WriteByte(cmd)
	buf.WriteByte(addrType)
	if addrType == 0x03 {
		buf.WriteByte(byte(len(addrBytes)))
	}
	buf.Write(addrBytes)
	_ = binary.Write(&buf, binary.BigEndian, uint16(port))
	return buf.Bytes()
}

// parseMetadata 从解密后的数据中解析元数据
func (a *Agent) parseMetadata(data []byte) (byte, string, error) {
	if len(data) < 2 {
		return 0, "", fmt.Errorf("metadata packet too short")
	}
	cmd := data[0]
	addrType := data[1]
	var host string
	var port int
	offset := 2

	switch addrType {
	case 0x01: // IPv4
		if len(data) < offset+4+2 {
			return 0, "", fmt.Errorf("invalid ipv4 metadata")
		}
		host = net.IP(data[offset : offset+4]).String()
		offset += 4
	case 0x03: // Domain
		domainLen := int(data[offset])
		offset++
		if len(data) < offset+domainLen+2 {
			return 0, "", fmt.Errorf("invalid domain metadata")
		}
		host = string(data[offset : offset+domainLen])
		offset += domainLen
	case 0x04: // IPv6
		if len(data) < offset+16+2 {
			return 0, "", fmt.Errorf("invalid ipv6 metadata")
		}
		host = net.IP(data[offset : offset+16]).String()
		offset += 16
	default:
		return 0, "", fmt.Errorf("unsupported address type in metadata: %d", addrType)
	}

	port = int(binary.BigEndian.Uint16(data[offset : offset+2]))
	return cmd, net.JoinHostPort(host, strconv.Itoa(port)), nil
}

// --- END OF NEW FILE ---

// GetTargetFromHTTPHeader 从一个bufio.Reader中解析出目标地址。
// 它处理HTTP CONNECT隧道和普通HTTP请求，是一个可导出的公共函数。
func GetTargetFromHTTPHeader(reader *bufio.Reader) (header []byte, hostname string, port int, isSSL bool, err error) {
	// 窥探足够多的数据以确保能读到完整的Host头
	header, err = reader.Peek(reader.Buffered())
	if err != nil && err != bufio.ErrBufferFull {
		return nil, "", 0, false, err
	}

	firstLineEnd := bytes.Index(header, []byte("\r\n"))
	if firstLineEnd == -1 {
		return nil, "", 0, false, net.InvalidAddrError("malformed http request")
	}
	firstLine := string(header[:firstLineEnd])

	if strings.HasPrefix(firstLine, "CONNECT") {
		isSSL = true
		parts := strings.Split(firstLine, " ")
		if len(parts) < 2 {
			return nil, "", 0, false, net.InvalidAddrError("malformed CONNECT request")
		}
		host, portStr, splitErr := net.SplitHostPort(parts[1])
		if splitErr != nil {
			hostname = parts[1]
			port = 443
		} else {
			hostname = host
			port, _ = strconv.Atoi(portStr)
		}
		return header, hostname, port, isSSL, nil
	}

	hostHeaderStart := bytes.Index(bytes.ToLower(header), []byte("\nhost: "))
	if hostHeaderStart == -1 {
		return nil, "", 0, false, net.InvalidAddrError("Host header not found")
	}
	hostHeaderStart += 7 // Skip "\nhost: "
	hostHeaderEnd := bytes.Index(header[hostHeaderStart:], []byte("\r\n"))
	if hostHeaderEnd == -1 {
		return nil, "", 0, false, net.InvalidAddrError("malformed Host header")
	}

	hostPort := string(header[hostHeaderStart : hostHeaderStart+hostHeaderEnd])
	host, portStr, splitErr := net.SplitHostPort(hostPort)
	if splitErr != nil {
		hostname = hostPort
		port = 80
	} else {
		hostname = host
		port, _ = strconv.Atoi(portStr)
	}

	return header, hostname, port, isSSL, nil
}
