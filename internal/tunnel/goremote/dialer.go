// --- START OF NEW FILE liuproxy_go/internal/goremote/dialer.go ---
package goremote

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"github.com/gorilla/websocket"
	"liuproxy_go/internal/shared"
	"liuproxy_go/internal/shared/logger"
	"net"
	"net/http"
	"net/url"
	"time"
)

// Dial 负责为 GoRemote 策略建立 WebSocket 连接。
func Dial(urlStr string) (net.Conn, error) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("goremote dial: invalid URL: %w", err)
	}

	// 为 GoRemote 设置一个独特的 User-Agent
	requestHeader := http.Header{}
	requestHeader.Set("Host", parsedURL.Hostname())
	requestHeader.Set("User-Agent", "LiuProxy-Client/2.0 (GoRemote)")

	dialer := *websocket.DefaultDialer
	dialer.HandshakeTimeout = 15 * time.Second

	// 自定义 NetDialContext 以支持 EdgeIP
	dialer.NetDialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		dialAddr := addr
		d := &net.Dialer{Timeout: 10 * time.Second}
		return d.DialContext(ctx, network, dialAddr)
	}

	// 为 wss 连接应用详细的 TLS 诊断
	if parsedURL.Scheme == "wss" {
		dialer.TLSClientConfig = &tls.Config{
			ServerName: parsedURL.Hostname(), // 设置SNI
			VerifyConnection: func(cs tls.ConnectionState) error {
				// 执行标准验证
				opts := x509.VerifyOptions{
					DNSName:       cs.ServerName,
					Intermediates: x509.NewCertPool(),
				}
				for _, cert := range cs.PeerCertificates[1:] {
					opts.Intermediates.AddCert(cert)
				}
				_, err := cs.PeerCertificates[0].Verify(opts)
				if err != nil {
					return err
				}
				return nil
			},
		}
	}

	ws, _, err := dialer.Dial(urlStr, requestHeader)
	if err != nil {
		return nil, err
	}

	logger.Debug().Msg("[GoRemote Dialer-Debug] SUCCESS: WebSocket connection established.")
	return shared.NewWebSocketConnAdapter(ws), nil
}

// DialWithEdgeIP (暂时保留，以防未来需要)

// --- END OF NEW FILE ---
