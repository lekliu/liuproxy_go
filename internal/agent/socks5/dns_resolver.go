// --- START OF COMPLETE REPLACEMENT for dns_resolver.go ---
// file: liuproxy_go/internal/agent/socks5/dns_resolver.go

package socks5

import (
	"context"
	"liuproxy_go/internal/core/securecrypt"
	"log"
	"net"
	"time"

	"golang.org/x/net/dns/dnsmessage"
	"liuproxy_go/internal/protocol"
)

// DNSResolver 负责处理DNS查询请求。
// 它现在是无状态的，并且不直接处理加密。
type DNSResolver struct {
	cipher *securecrypt.Cipher // 保持cipher字段，因为旧的DNS响应逻辑需要它
}

// NewDNSResolver 创建一个新的DNSResolver实例。
func NewDNSResolver(cipher *securecrypt.Cipher) *DNSResolver {
	return &DNSResolver{
		cipher: cipher,
	}
}

// HandleDNSRequest 是处理DNS查询的入口函数。
// 它接收一个RemoteTunnel实例，用于回传响应。
func (d *DNSResolver) HandleDNSRequest(requestPayload []byte, tunnel *RemoteTunnel, streamID uint16, originalRequestPayload []byte) {
	var request dnsmessage.Message
	if err := request.Unpack(requestPayload); err != nil {
		return
	}

	if len(request.Questions) == 0 {
		return
	}

	question := request.Questions[0]
	domainName := question.Name.String()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", domainName)

	if err != nil {
		response := dnsmessage.Message{
			Header: dnsmessage.Header{
				ID:       request.ID,
				Response: true,
				RCode:    dnsmessage.RCodeServerFailure,
			},
			Questions: []dnsmessage.Question{question},
		}
		d.sendResponse(response, tunnel, streamID, originalRequestPayload)
		return
	}

	response := dnsmessage.Message{
		Header: dnsmessage.Header{
			ID:            request.ID,
			Response:      true,
			Authoritative: true,
		},
		Questions: []dnsmessage.Question{question},
	}

	for _, ip := range ips {
		if ip4 := ip.To4(); ip4 != nil {
			response.Answers = append(response.Answers, dnsmessage.Resource{
				Header: dnsmessage.ResourceHeader{
					Name:  question.Name,
					Type:  dnsmessage.TypeA,
					Class: dnsmessage.ClassINET,
					TTL:   60,
				},
				Body: &dnsmessage.AResource{A: [4]byte(ip4)},
			})
		}
	}

	d.sendResponse(response, tunnel, streamID, originalRequestPayload)
}

// sendResponse 负责将构建好的DNS响应打包，并交由隧道发送（隧道会负责加密）。
func (d *DNSResolver) sendResponse(response dnsmessage.Message, tunnel *RemoteTunnel, streamID uint16, originalRequestPayload []byte) {
	responsePayload, err := response.Pack()
	if err != nil {
		return
	}

	// 从原始请求中提取SOCKS5头部
	if len(originalRequestPayload) < 10 {
		return
	}
	socks5Header := originalRequestPayload[0:10]

	// 将SOCKS5头部与DNS响应拼接，形成完整的明文SOCKS5 UDP包
	finalPayload := append(socks5Header, responsePayload...)

	// 将明文包封装成隧道协议包
	packet := protocol.Packet{
		StreamID: streamID,
		Flag:     protocol.FlagUDPData,
		Payload:  finalPayload, // Payload是明文
	}

	// 交由隧道发送（隧道会自动加密）
	if err := tunnel.WritePacket(&packet); err != nil {
		log.Printf("[RemoteDNS] FAILED to write DNS response to tunnel: %v", err)
	}
}
