// 修改点 1: 恢复了整个函数的逻辑，使用 for 循环来读取和解析头部 **********
// Modification Point 1: Restored the entire function's logic to use a for loop for reading and parsing the header **********
// 原始行号: 17
package agent

import (
	"bytes"
	"net"
	"strconv"
	"strings"
	"time"

	"liuproxy_go/internal/core/crypt"
)

// getTargetFromHTTPHeader 是一个被 HTTP 和 WebSocket agent 共享的辅助函数，
// 用于从入站连接中解析出最终的目标地址。
func getTargetFromHTTPHeader(conn net.Conn, bufferSize int, decrypt bool, encryptKey int) (header []byte, hostname string, port int, isSSL bool) {
	var accumulatedHeader []byte
	buf := make([]byte, bufferSize)

	for {
		// 为每一次 Read 操作设置超时
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, err := conn.Read(buf)
		if err != nil {
			// 读取超时或失败，终止
			_ = conn.SetReadDeadline(time.Time{})
			return nil, "", 0, false
		}

		accumulatedHeader = append(accumulatedHeader, buf[:n]...)

		var dataToParse []byte
		// 解密逻辑只对 Remote 端有效，并且只在第一次解密时有意义
		// 为了忠实于原逻辑，我们在每次循环都尝试解密
		if decrypt {
			dataToParse = crypt.DecryptWithHeader(accumulatedHeader, encryptKey)
		} else {
			dataToParse = accumulatedHeader
		}

		if len(dataToParse) > 0 {
			firstLine := strings.Split(string(dataToParse), "\n")[0]
			if strings.Contains(firstLine, "CONNECT") {
				isSSL = true
				hostPort := strings.Split(firstLine, " ")[1]
				host, portStr, err := net.SplitHostPort(hostPort)
				if err != nil {
					hostname = hostPort
					port = 443
				} else {
					hostname = host
					port, _ = strconv.Atoi(portStr)
				}
				_ = conn.SetReadDeadline(time.Time{})
				return accumulatedHeader, hostname, port, isSSL
			}

			hostIndex := bytes.Index(dataToParse, []byte("Host:"))
			if hostIndex > -1 {
				hostLineEnd := bytes.Index(dataToParse[hostIndex:], []byte("\n"))
				if hostLineEnd == -1 {
					// 找到了 Host: 但还没找到换行符，继续读取
					continue
				}
				hostLine := string(dataToParse[hostIndex+5 : hostIndex+hostLineEnd])
				hostPort := strings.TrimSpace(hostLine)
				host, portStr, err := net.SplitHostPort(hostPort)
				if err != nil {
					hostname = hostPort
					port = 80
				} else {
					hostname = host
					port, _ = strconv.Atoi(portStr)
				}
				_ = conn.SetReadDeadline(time.Time{})
				return accumulatedHeader, hostname, port, isSSL
			}
		}
	}
}
