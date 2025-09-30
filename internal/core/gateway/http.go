package gateway

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"net/http"
	"strings"
)

// sniffTargetHTTP 嗅探 Host 并确保始终返回 host:port 格式。
func sniffTargetHTTP(conn net.Conn, reader *bufio.Reader) (string, *http.Request, error) {
	buffered := reader.Buffered()
	if buffered == 0 {
		// 确保至少有一个字节可供读取
		if _, err := reader.Peek(1); err != nil {
			return "", nil, fmt.Errorf("failed to peek for http sniff: %w", err)
		}
		buffered = reader.Buffered()
	}

	data, _ := reader.Peek(buffered)
	req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(data)))
	if err != nil {
		return "", nil, fmt.Errorf("could not parse HTTP request: %w", err)
	}

	host := req.Host
	if host == "" {
		return "", req, fmt.Errorf("HTTP request host is empty")
	}

	// 关键修复：检查 Host 是否已包含端口，如果没有，则根据协议补充默认端口。
	if !strings.Contains(host, ":") {
		if req.Method == "CONNECT" {
			host = net.JoinHostPort(host, "443") // HTTPS 默认端口
		} else {
			host = net.JoinHostPort(host, "80") // HTTP 默认端口
		}
	}

	return host, req, nil
}
