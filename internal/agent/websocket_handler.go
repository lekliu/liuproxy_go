// 修改点 1: 移除了在文件顶部的全局 upgrader 变量。它将在 handleRemote 中动态创建。 **********
// Modification Point 1: Removed the global upgrader variable at the top of the file. It will be created dynamically in handleRemote. **********
// 原始行号: 18
package agent

import (
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"liuproxy_go/internal/core/crypt"
	"liuproxy_go/internal/shared"
	"liuproxy_go/pkg/geoip"
)

// handleLocal 处理来自浏览器的连接，并建立到 remote 的 WebSocket 隧道
func (a *WebSocketAgent) handleLocal(inboundConn net.Conn) {
	defer inboundConn.Close()

	// --- Handshake 阶段 (Local) ---
	// 1. 解析浏览器请求
	initialData, hostname, port, isSSL := getTargetFromHTTPHeader(inboundConn, a.bufferSize, false, 0)
	if hostname == "" {
		return
	}

	// 2. GeoIP 检查
	switch geoip.GeoIP(hostname) {
	case 0: // 断开
		return
	case 1: // 直连
		directAgent := NewDirectAgent(hostname, port, isSSL, initialData, a.bufferSize)
		directAgent.HandleConnection(inboundConn)
		return
	}

	// 3. 连接到远程 WebSocket 服务器
	u := url.URL{Scheme: a.config.RemoteScheme, Host: net.JoinHostPort(a.config.RemoteHost, strconv.Itoa(a.config.RemotePort)), Path: "/ws"}
	wsConn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return
	}
	defer wsConn.Close()

	// 4. 发送元数据
	targetAddress := net.JoinHostPort(hostname, strconv.Itoa(port))
	meta := shared.ProxyRequest{Address: targetAddress}
	jsonData, _ := json.Marshal(meta)
	if err := wsConn.WriteMessage(websocket.TextMessage, jsonData); err != nil {
		return
	}

	// 5. 等待 remote 确认
	_, confirmMsg, err := wsConn.ReadMessage()
	if err != nil || !strings.HasPrefix(string(confirmMsg), "success") {
		return
	}

	// 6. 响应浏览器并转发初始数据
	if isSSL {
		inboundConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	} else {
		encryptedHeader := crypt.Encrypt(initialData, a.config.EncryptKey)
		if err := wsConn.WriteMessage(websocket.BinaryMessage, encryptedHeader); err != nil {
			return
		}
	}
	// --- Handshake 结束 ---

	// --- Relay 阶段 ---
	relayCfg := RelayConfig{
		IsServerSide: a.config.IsServerSide,
		EncryptKey:   a.config.EncryptKey,
		BufferSize:   a.bufferSize,
	}
	// Local 端:
	// inbound (tcp) -> outbound (ws) 是上行流，需要加密
	tcpToWsTransformer := getTransformer(false, true, relayCfg.EncryptKey)
	// outbound (ws) -> inbound (tcp) 是下行流，需要解密
	wsToTcpTransformer := getTransformer(false, false, relayCfg.EncryptKey)

	WebSocketRelay(inboundConn, wsConn, tcpToWsTransformer, wsToTcpTransformer, relayCfg)
}

// handleRemote 处理来自 local 的 WebSocket 连接请求
func (a *WebSocketAgent) handleRemote(w http.ResponseWriter, r *http.Request) {
	// 修改点 2: 在函数内部根据配置动态创建 upgrader，这是解决 panic 的关键修复 **********
	// Modification Point 2: Dynamically create the upgrader inside the function based on the configuration. This is the key fix for the panic. **********
	// 原始行号: (新逻辑 / New Logic)
	upgrader := websocket.Upgrader{
		ReadBufferSize:  a.bufferSize,
		WriteBufferSize: a.bufferSize,
		CheckOrigin:     func(r *http.Request) bool { return true },
	}

	// --- Handshake 阶段 (Remote) ---
	// 1. 升级 HTTP 连接到 WebSocket
	wsConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer wsConn.Close()

	// 2. 读取元数据
	_, msg, err := wsConn.ReadMessage()
	if err != nil {
		return
	}
	var req shared.ProxyRequest
	if err := json.Unmarshal(msg, &req); err != nil {
		return
	}

	// 3. 连接到最终目标
	outboundConn, err := net.DialTimeout("tcp", req.Address, 10*time.Second)
	if err != nil {
		wsConn.WriteMessage(websocket.TextMessage, []byte("error: connection failed"))
		return
	}
	defer outboundConn.Close()

	// 4. 发送成功确认
	wsConn.WriteMessage(websocket.TextMessage, []byte("success: connected"))
	// --- Handshake 结束 ---

	// --- Relay 阶段 ---
	relayCfg := RelayConfig{
		IsServerSide: a.config.IsServerSide,
		EncryptKey:   a.config.EncryptKey,
		BufferSize:   a.bufferSize,
	}
	// Remote 端:
	// outbound (tcp) -> inbound (ws) 是下行流，需要加密
	tcpToWsTransformer := getTransformer(true, false, relayCfg.EncryptKey)
	// inbound (ws) -> outbound (tcp) 是上行流，需要解密
	wsToTcpTransformer := getTransformer(true, true, relayCfg.EncryptKey)

	WebSocketRelay(outboundConn, wsConn, tcpToWsTransformer, wsToTcpTransformer, relayCfg)
}
