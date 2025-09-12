// --- START OF COMPLETE REPLACEMENT for liuproxy_go/mobile/api.go ---
package mobile

import (
	"encoding/json"
	"fmt"
	"liuproxy_go/internal/types"
	"log"
	"net"
	"strings"
	"sync"

	"liuproxy_go/internal/server"
)

var (
	appServerInstance *server.AppServer
	instanceMutex     sync.Mutex
)

// MobileConfig 用于解析从Android端传递过来的JSON配置
type MobileConfig struct {
	Key                      int    `json:"key"`
	BufferSize               int    `json:"buffer_size"`
	PreformattedServerString string `json:"preformatted_server_string"`
}

func init() {
	// This helps distinguish Go logs in logcat
	log.SetPrefix("[GoLiuProxy] ")
}

// StartVPN 启动 Go 核心的 local 代理逻辑。
// 它会动态创建一个SOCKS5/HTTP统一监听器，并返回其监听的端口号。
func StartVPN(configJSON string) (int64, error) {
	instanceMutex.Lock()
	defer instanceMutex.Unlock()

	if appServerInstance != nil {
		return 0, fmt.Errorf("service is already running")
	}

	log.Println("Configuring and starting Go core for mobile...")

	// 1. 解析JSON配置
	var mobileCfg MobileConfig
	if err := json.Unmarshal([]byte(configJSON), &mobileCfg); err != nil {
		return 0, fmt.Errorf("配置解析错误: %w", err)
	}

	// 将接收到的字符串直接分割成 []string
	serverParts := strings.Split(mobileCfg.PreformattedServerString, ",")

	// 验证字段数量
	if len(serverParts) < 8 {
		return 0, fmt.Errorf("preformatted_server_string must contain at least 8 elements, got %d", len(serverParts))
	}

	// 2. 根据解析出的配置构建Go核心所需的 types.Config
	cfg := &types.Config{
		CommonConf: types.CommonConf{
			Mode:           "local",
			MaxConnections: 16,
			BufferSize:     int(mobileCfg.BufferSize),
			Crypt:          int(mobileCfg.Key),
		},
		LocalConf: types.LocalConf{
			UnifiedPort: 0, // 动态选择端口
			// 构建远程服务器配置数组
			RemoteIPs: [][]string{
				// 我们总是激活移动端传递的唯一配置
				serverParts,
			},
		},
	}

	// 3. 创建并启动服务器
	appServer := server.New(cfg, "")
	// --- Capture error from RunMobile more effectively ---
	// RunMobile now directly returns the listener or an error.
	listener, err := appServer.RunMobile()
	if err != nil {
		return 0, fmt.Errorf("failed to run mobile server: %w", err)
	}

	// 4. 获取动态分配的端口
	var unifiedPort int
	if listener != nil && listener.PortType == "unified" {
		if tcpAddr, ok := listener.Listener.Addr().(*net.TCPAddr); ok {
			unifiedPort = tcpAddr.Port
		} else {
			appServer.Stop()
			return 0, fmt.Errorf("listener is not a TCP listener")
		}
	} else {
		appServer.Stop()
		return 0, fmt.Errorf("could not find the unified listener after startup")
	}

	// 5. 在后台等待服务器完全停止
	go appServer.Wait()

	// 6. 保存实例并返回成功
	appServerInstance = appServer
	return int64(unifiedPort), nil
}

// StopVPN 停止 Go 核心。
func StopVPN() {
	instanceMutex.Lock()
	defer instanceMutex.Unlock()

	if appServerInstance != nil {
		appServerInstance.Stop()
		appServerInstance = nil
	}
}

// --- END OF COMPLETE REPLACEMENT ---
