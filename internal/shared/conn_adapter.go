// --- START OF FINAL FIX for liuproxy_go/internal/shared/conn_adapter.go ---
package shared

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url" // 1/2 新增: 导入 net/url 包
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// ... upgrader 和 WebSocketConnAdapter 结构体保持不变 ...
var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

type WebSocketConnAdapter struct {
	*websocket.Conn
	readBuffer *ThreadSafeBuffer
}

func NewWebSocketConnAdapterServer(w http.ResponseWriter, r *http.Request) (*WebSocketConnAdapter, error) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return nil, err
	}
	return &WebSocketConnAdapter{
		Conn:       ws,
		readBuffer: NewThreadSafeBuffer(),
	}, nil
}

// --- 1/1 MODIFICATION START: Explicitly set Host header ---
func NewWebSocketConnAdapterClient(urlStr string) (*WebSocketConnAdapter, error) {
	log.Printf("[WSC_Client] Attempting to dial: %s", urlStr)

	// 1. 解析URL以获取Host
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid URL for websocket: %w", err)
	}
	// a. 对于带端口的地址，Host是 "domain:port"
	// b. 对于标准端口 (ws:80, wss:443)，Host是 "domain"
	// gorilla/websocket库期望Host头不包含标准端口，所以我们只取hostname
	requestHeader := http.Header{}
	requestHeader.Set("Host", parsedURL.Hostname())

	dialer := *websocket.DefaultDialer
	if strings.HasPrefix(urlStr, "wss://") {
		dialer.TLSClientConfig = &tls.Config{
			// 我们仍然保留这个，因为它对某些自签名证书或中间人网络环境有好处
			InsecureSkipVerify: true,
			// 明确设置SNI，这对于多域名服务器至关重要
			ServerName: parsedURL.Hostname(),
		}
		log.Printf("[WSC_Client] Using custom TLS dialer for wss. Host header: '%s', SNI: '%s'", requestHeader.Get("Host"), parsedURL.Hostname())
	} else {
		log.Printf("[WSC_Client] Using default dialer for ws. Host header: '%s'", requestHeader.Get("Host"))
	}

	ws, resp, err := dialer.Dial(urlStr, requestHeader)

	if err != nil {
		log.Printf("[WSC_Client] FAILED to dial. Error: %v", err)
		if resp != nil {
			body, _ := io.ReadAll(resp.Body)
			log.Printf("[WSC_Client] Server Response Status: %s", resp.Status)
			log.Printf("[WSC_Client] Server Response Body:\n--- START RESPONSE ---\n%s\n--- END RESPONSE ---", string(body))
		}
		return nil, err
	}

	log.Println("[WSC_Client] SUCCESS: Connection established.")
	return &WebSocketConnAdapter{
		Conn:       ws,
		readBuffer: NewThreadSafeBuffer(),
	}, nil
}

// NewWebSocketConnAdapterClientWithEdgeIP 也应用相同的修复
func NewWebSocketConnAdapterClientWithEdgeIP(urlStr string, edgeIP string) (*WebSocketConnAdapter, error) {
	log.Printf("[WSC_EdgeIP] Attempting to dial. URL: %s, EdgeIP: %s", urlStr, edgeIP)

	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid URL for websocket: %w", err)
	}
	requestHeader := http.Header{}
	requestHeader.Set("Host", parsedURL.Hostname())

	dialer := *websocket.DefaultDialer

	if strings.HasPrefix(urlStr, "wss://") {
		dialer.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         parsedURL.Hostname(),
		}
	}

	dialer.NetDialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		_, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		edgeAddr := net.JoinHostPort(edgeIP, port)
		log.Printf("[WSC_EdgeIP] Dialing via Edge IP: %s (Original addr: %s)", edgeAddr, addr)
		d := &net.Dialer{}
		return d.DialContext(ctx, network, edgeAddr)
	}

	ws, resp, err := dialer.Dial(urlStr, requestHeader)
	if err != nil {
		log.Printf("[WSC_EdgeIP] FAILED to dial with Edge IP. Error: %v", err)
		if resp != nil {
			body, _ := io.ReadAll(resp.Body)
			log.Printf("[WSC_EdgeIP] Server Response Status: %s", resp.Status)
			log.Printf("[WSC_EdgeIP] Server Response Body:\n--- START RESPONSE ---\n%s\n--- END RESPONSE ---", string(body))
		}
		return nil, err
	}

	log.Println("[WSC_EdgeIP] SUCCESS: Connection with Edge IP established.")
	return &WebSocketConnAdapter{
		Conn:       ws,
		readBuffer: NewThreadSafeBuffer(),
	}, nil
}

// --- 1/1 MODIFICATION END ---

// ... Read, Write, Close等其他函数保持不变 ...
func (wsc *WebSocketConnAdapter) Read(b []byte) (int, error) {
	if wsc.readBuffer.Len() == 0 {
		msgType, msg, err := wsc.Conn.ReadMessage()
		if err != nil {
			return 0, err
		}

		if msgType != websocket.BinaryMessage {
			return 0, fmt.Errorf("received non-binary message from websocket")
		}
		if _, err := wsc.readBuffer.Write(msg); err != nil {
			return 0, err
		}
	}
	return wsc.readBuffer.Read(b)
}
func (wsc *WebSocketConnAdapter) Write(b []byte) (int, error) {
	dataCopy := make([]byte, len(b))
	copy(dataCopy, b)

	err := wsc.Conn.WriteMessage(websocket.BinaryMessage, dataCopy)
	if err != nil {
		return 0, err
	}
	return len(b), nil
}
func (wsc *WebSocketConnAdapter) Close() error {
	return wsc.Conn.Close()
}
func (wsc *WebSocketConnAdapter) LocalAddr() net.Addr {
	return wsc.Conn.LocalAddr()
}
func (wsc *WebSocketConnAdapter) RemoteAddr() net.Addr {
	return wsc.Conn.RemoteAddr()
}
func (wsc *WebSocketConnAdapter) SetDeadline(t time.Time) error {
	_ = wsc.Conn.SetReadDeadline(t)
	return wsc.Conn.SetWriteDeadline(t)
}
func (wsc *WebSocketConnAdapter) SetReadDeadline(t time.Time) error {
	return wsc.Conn.SetReadDeadline(t)
}
func (wsc *WebSocketConnAdapter) SetWriteDeadline(t time.Time) error {
	return wsc.Conn.SetWriteDeadline(t)
}

// --- END OF FINAL FIX ---
