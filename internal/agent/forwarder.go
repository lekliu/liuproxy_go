// 修改点 1: 导入 "fmt" 包 **********
// Modification Point 1: Import the "fmt" package **********
// 原始行号: 5
package agent

import (
	"github.com/gorilla/websocket"
	"net"
	"sync"

	"liuproxy_go/internal/core/crypt"
)

// RelayConfig 定义了转发器需要的配置
type RelayConfig struct {
	IsServerSide bool
	EncryptKey   int
	BufferSize   int
}

// TCPRelay 在两个 TCP 连接之间进行数据转发。
// 它是现在所有 TCP 代理（HTTP, SOCKS5）的唯一转发器。
func TCPRelay(conn1, conn2 net.Conn, cfg RelayConfig) {
	var wg sync.WaitGroup
	wg.Add(2)

	// conn1: inbound (来自浏览器或local), conn2: outbound (发往remote或target)
	localToRemoteTransformer := getTransformer(cfg.IsServerSide, true, cfg.EncryptKey)
	remoteToLocalTransformer := getTransformer(cfg.IsServerSide, false, cfg.EncryptKey)

	side := "LOCAL"
	if cfg.IsServerSide {
		side = "REMOTE"
	}

	// conn1: inbound, conn2: outbound
	go func() {
		defer wg.Done()
		transfer(conn2, conn1, localToRemoteTransformer, cfg.BufferSize, side, "IN->OUT")
	}()

	go func() {
		defer wg.Done()
		transfer(conn1, conn2, remoteToLocalTransformer, cfg.BufferSize, side, "OUT->IN")
	}()

	wg.Wait()
}

// WebSocketRelay 是专门为 WebSocket 设计的转发器
func WebSocketRelay(tcpConn net.Conn, wsConn *websocket.Conn, tcpToWsTransformer, wsToTcpTransformer func([]byte) []byte, cfg RelayConfig) {
	var wg sync.WaitGroup
	wg.Add(2)

	// tcp -> ws
	go func() {
		defer wg.Done()
		buf := make([]byte, cfg.BufferSize)
		for {
			n, err := tcpConn.Read(buf)
			if err != nil {
				wsConn.Close()
				return
			}
			transformedData := tcpToWsTransformer(buf[:n])
			if err := wsConn.WriteMessage(websocket.BinaryMessage, transformedData); err != nil {
				return
			}
		}
	}()

	// ws -> tcp
	go func() {
		defer wg.Done()
		for {
			_, data, err := wsConn.ReadMessage()
			if err != nil {
				tcpConn.Close()
				return
			}
			transformedData := wsToTcpTransformer(data)
			if _, err := tcpConn.Write(transformedData); err != nil {
				return
			}
		}
	}()
	wg.Wait()
}

// getTransformer 根据角色和方向返回正确的加/解密函数。
// isLocalToRemote: 数据流向是否是从客户端 -> 服务器
func getTransformer(isServerSide, isLocalToRemote bool, key int) func([]byte) []byte {
	if isServerSide { // Remote 端
		if isLocalToRemote { // 数据从 Local 来 (Inbound)
			return func(b []byte) []byte { return crypt.Decrypt(b, key) }
		} else { // 数据往 Local 去 (Outbound)
			return func(b []byte) []byte { return crypt.Encrypt(b, key) }
		}
	} else { // Local 端
		if isLocalToRemote { // 数据往 Remote 去 (Outbound)
			return func(b []byte) []byte { return crypt.Encrypt(b, key) }
		} else { // 数据从 Remote 来 (Inbound)
			return func(b []byte) []byte { return crypt.Decrypt(b, key) }
		}
	}
}

// transfer 是单向的、可配置的 TCP 数据流处理函数
func transfer(dst net.Conn, src net.Conn, transformer func([]byte) []byte, bufferSize int, side string, direction string) {
	defer dst.Close()
	defer src.Close()
	buf := make([]byte, bufferSize)
	for {
		n, err := src.Read(buf)
		if err != nil {
			return
		}
		dataToTransform := buf[:n]
		dataToWrite := transformer(dataToTransform)

		if _, err := dst.Write(dataToWrite); err != nil {
			return
		}
	}
}
