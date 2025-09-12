// --- START OF COMPLETE REPLACEMENT for types.go ---
package types

import (
	"bufio"
	"net"
)

// Agent 接口定义了所有代理处理器的通用行为。
// HandleConnection 接收一个原始连接和一个可能已预读数据的 bufio.Reader。
// 如果 reader 为 nil，处理器应从 conn 创建自己的 reader。
// 如果 reader 不为 nil，处理器必须使用这个 reader 来读取初始数据。
type Agent interface {
	HandleConnection(conn net.Conn, reader *bufio.Reader)
}

// --- 1/1 MODIFICATION START ---
// 将所有配置结构体统一到这里，并添加 ini 标签

// CommonConf 包含 local 和 remote 模式共有的配置
type CommonConf struct {
	Mode           string `ini:"mode"`
	MaxConnections int    `ini:"maxConnections"`
	BufferSize     int    `ini:"bufferSize"`
	Crypt          int    `ini:"crypt"`
}

// RemoteConf 包含 remote 模式特有的配置
type RemoteConf struct {
	PortWsSvr int `ini:"port_ws_svr"`
}

// LocalConf 包含 local 模式特有的配置
type LocalConf struct {
	UnifiedPort int `ini:"unified_port"`
	WebPort     int `ini:"web_port"`
	// RemoteIPs 字段由 LoadIni 函数手动解析，因此不需要 ini 标签
	// 格式: [["host", "port", "scheme", "path", "type", "edge_ip"], ...]
	RemoteIPs [][]string `ini:"-"`
}

// Config 是整个应用程序的统一配置结构体
type Config struct {
	CommonConf `ini:"common"`
	LocalConf  `ini:"local"`
	RemoteConf `ini:"remote"`
}
