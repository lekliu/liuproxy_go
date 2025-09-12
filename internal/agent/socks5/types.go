// --- START OF NEW FILE internal/agent/socks5/types.go ---
package socks5

import (
	"net"
	"time"
)

// udpTimeout 定义了 UDP 会话的超时时间
const udpTimeout = 60 * time.Second

// toRemoteQueueSize 定义了 Local 端 UDP 包缓存的大小
const toRemoteQueueSize = 1024

// udpPacket 定义了在缓存 channel 中传递的数据结构
type udpPacket struct {
	data []byte   // SOCKS5 UDP 请求包 (带头部)
	addr net.Addr // 客户端的公-网地址 (e.g., 127.0.0.1:port)
}
