// FILE: liuproxy_go\internal\agent\socks5_udp_handler.go
// ---------------- START OF NEW FILE ----------------

package agent

import (
	"fmt"
	"io"
	"liuproxy_go/internal/core/crypt"
	"net"
	"sync"
	"time"
)

const udpTimeout = 60 * time.Second

// -------------------------------------------------------------------
// Part 1: Remote Side Logic (处理来自 Local 端的 UDP ASSOCIATE 请求)
// -------------------------------------------------------------------

// UDPRelay 在 remote 端管理所有 UDP 会话
type UDPRelay struct {
	listener   net.PacketConn      // Remote 端的 UDP 监听器
	sessions   map[string]net.Conn // 会话映射: "clientAddr" -> targetConn
	lock       sync.Mutex
	encryptKey int
	bufferSize int
}

// handleUdpAssociateRemote 是 remote 端处理 UDP ASSOCIATE 命令的入口
// 由 socks5_handler.go 调用
func (a *Socks5Agent) handleUdpAssociateRemote(tcpConn net.Conn) {
	udpListener, err := net.ListenPacket("udp", "0.0.0.0:0")
	if err != nil {
		return
	}
	defer udpListener.Close()

	udpAddr := udpListener.LocalAddr().(*net.UDPAddr)

	// 构造一个标准的、10字节的 SOCKS5 IPv4 响应
	reply := []byte{
		0x05, // VER
		0x00, // REP (Success)
		0x00, // RSV
		0x01, // ATYP (IPv4)
	}
	// 不再使用 udpAddr.IP.To4()，因为它可能返回 nil。
	// SOCKS5规范允许在这里返回 0.0.0.0。
	// local 端实际上关心的是端口号，IP地址它会用配置文件里的。
	reply = append(reply, []byte{0, 0, 0, 0}...)
	reply = append(reply, byte(udpAddr.Port>>8), byte(udpAddr.Port&0xff))

	encryptedReply := crypt.Encrypt(reply, a.config.EncryptKey)
	if _, err := tcpConn.Write(encryptedReply); err != nil {
		return
	}

	// 3. 创建并运行 UDP 中继器
	relay := &UDPRelay{
		listener:   udpListener,
		sessions:   make(map[string]net.Conn),
		encryptKey: a.config.EncryptKey,
		bufferSize: a.bufferSize,
	}
	go relay.runForwardLoop()

	// 4. 保持 TCP 控制连接存活，如果断开，则说明客户端已离线
	// 通过不断读取来检测连接是否断开，读取到的数据被丢弃
	io.Copy(io.Discard, tcpConn)
	// 当这个函数返回时，udpListener 会被 defer 关闭，从而终止 runForwardLoop 中的 ReadFrom 阻塞
}

// runForwardLoop 是 remote UDP 中继的核心循环
func (relay *UDPRelay) runForwardLoop() {
	buf := make([]byte, relay.bufferSize)
	for {
		// 从 local 端读取一个加密的 SOCKS5 UDP 包
		n, fromAddr, err := relay.listener.ReadFrom(buf)
		if err != nil {
			break
		}

		decryptedData := crypt.Decrypt(buf[:n], relay.encryptKey)

		// 解析SOCKS5 UDP请求头 (RSV, FRAG, ATYP, DST.ADDR, DST.PORT)
		if len(decryptedData) < 10 {
			continue
		}
		// RSV (2 bytes), FRAG (1 byte)
		if decryptedData[2] != 0x00 { // 不支持分片
			continue
		}

		// 这里简化处理，只支持 IPv4
		targetHost := net.IP(decryptedData[4:8]).String()
		targetPort := (int(decryptedData[8]) << 8) + int(decryptedData[9])
		targetAddr := fmt.Sprintf("%s:%d", targetHost, targetPort)
		payload := decryptedData[10:]
		clientKey := fromAddr.String()

		relay.lock.Lock()
		targetConn, found := relay.sessions[clientKey]
		if !found {
			// 新会话，连接到最终目标
			conn, dialErr := net.DialTimeout("udp", targetAddr, 10*time.Second)
			if dialErr != nil {
				relay.lock.Unlock()
				continue
			}
			targetConn = conn
			relay.sessions[clientKey] = targetConn

			// 为这个新连接启动一个goroutine，用于接收来自目标的数据
			go relay.copyFromTarget(targetConn, fromAddr)
		}
		relay.lock.Unlock()

		// 转发数据到最终目标
		targetConn.Write(payload)
		// 每次写入都刷新超时时间
		targetConn.SetReadDeadline(time.Now().Add(udpTimeout))
	}
}

// copyFromTarget 从最终目标读取数据，并发回给 local 端
func (relay *UDPRelay) copyFromTarget(targetConn net.Conn, clientAddr net.Addr) {
	// 清理工作
	defer func() {
		relay.lock.Lock()
		delete(relay.sessions, clientAddr.String())
		targetConn.Close()
		relay.lock.Unlock()
	}()

	buf := make([]byte, relay.bufferSize)
	for {
		// 每次循环前都设置超时
		targetConn.SetReadDeadline(time.Now().Add(udpTimeout))
		n, err := targetConn.Read(buf)
		if err != nil {
			return // 连接超时或关闭
		}

		// 将目标地址和端口封装回 SOCKS5 UDP 头部
		// 对于 UDP 来说，我们实际上不需要告诉客户端源地址，所以可以简化
		header := []byte{0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}

		responsePacket := append(header, buf[:n]...)
		encryptedResponse := crypt.Encrypt(responsePacket, relay.encryptKey)

		// 将响应发回给 local 端
		relay.listener.WriteTo(encryptedResponse, clientAddr)
	}
}

// -------------------------------------------------------------------
// Part 2: Local Side Logic (处理来自客户端应用的 UDP ASSOCIATE 请求)
// -------------------------------------------------------------------

// handleUdpAssociateLocal 是 local 端处理 UDP 请求的入口
// 由 socks5_handler.go 调用
func (a *Socks5Agent) handleUdpAssociateLocal(clientTcpConn, remoteTcpConn net.Conn, remoteUdpAddr net.Addr) {
	// 1. 创建本地UDP监听器，让客户端把数据发到这里
	localListener, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		clientTcpConn.Close()
		remoteTcpConn.Close()
		return
	}
	defer localListener.Close()

	// 2. 构造唯一正确的SOCKS5响应，告诉客户端我们本地的UDP地址
	localUdpAddr := localListener.LocalAddr().(*net.UDPAddr)
	reply := []byte{0x05, 0x00, 0x00, 0x01} // VER, REP, RSV, ATYP(IPv4)
	reply = append(reply, localUdpAddr.IP.To4()...)
	reply = append(reply, byte(localUdpAddr.Port>>8), byte(localUdpAddr.Port&0xff))

	// 3. 发送这个响应给客户端
	if _, err := clientTcpConn.Write(reply); err != nil {
		clientTcpConn.Close()
		remoteTcpConn.Close()
		return
	}

	// 4. 创建一个专门用于和 remote UDP端口 通信的UDP连接
	connToRemote, err := net.DialUDP("udp", nil, remoteUdpAddr.(*net.UDPAddr))
	if err != nil {
		clientTcpConn.Close()
		remoteTcpConn.Close()
		return
	}
	defer connToRemote.Close()

	var clientAppAddr net.Addr
	var once sync.Once

	// Goroutine 1: 从客户端应用读 -> 加密 -> 发到 remote
	go func() {
		buf := make([]byte, a.bufferSize)
		for {
			n, addr, err := localListener.ReadFrom(buf)
			if err != nil {
				clientTcpConn.Close() // 触发下方阻塞解除
				remoteTcpConn.Close()
				return
			}
			once.Do(func() { clientAppAddr = addr }) // 记录第一个包的源地址
			encrypted := crypt.Encrypt(buf[:n], a.config.EncryptKey)
			connToRemote.Write(encrypted)
		}
	}()

	// Goroutine 2: 从 remote 读 -> 解密 -> 发到客户端应用
	go func() {
		buf := make([]byte, a.bufferSize)
		for {
			n, _, err := connToRemote.ReadFrom(buf)
			if err != nil {
				clientTcpConn.Close()
				remoteTcpConn.Close()
				return
			}
			if clientAppAddr != nil {
				decrypted := crypt.Decrypt(buf[:n], a.config.EncryptKey)
				localListener.WriteTo(decrypted, clientAppAddr)
			}
		}
	}()

	// 阻塞，直到客户端的TCP控制连接断开，以此来结束整个UDP会话
	io.Copy(io.Discard, clientTcpConn)
	remoteTcpConn.Close()
}

// getRemoteUDPConn 返回一个共享的UDP连接到remote，以避免每次都Dial
// (我们将在下一步添加到Socks5Agent中)
func (c *Config) getRemoteUDPConn() net.PacketConn {
	// 这是一个占位符，我们需要在Socks5Agent或Config层面实现一个单例的UDP连接
	// 为简单起见，我们暂时每次都创建，但在实际应用中应复用
	conn, _ := net.ListenPacket("udp", ":0")
	return conn
}
