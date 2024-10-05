package local

import (
	"fmt"
	"main/utils/data"
	"net"
	"sync"
)

type MySocks5 struct {
	connection net.Conn
	addr       net.Addr
	remoteid   int
	remoteIP   string
	remotePort int
	connDst    net.Conn
}

func NewMySocks5(conn net.Conn, addr net.Addr, remoteid int) *MySocks5 {
	return &MySocks5{
		connection: conn,
		addr:       addr,
		remoteid:   remoteid,
		remoteIP:   GetRemoteIP(remoteid),    // 假设有获取远程 IP 的函数
		remotePort: GetRemotePort2(remoteid), // 假设有获取远程端口的函数
	}
}

func (s *MySocks5) Start() {
	// Only supports CONNECT request
	dstConn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", s.remoteIP, s.remotePort))
	if err != nil {
		fmt.Println("Connection error:", err)
		return
	}
	s.connDst = dstConn

	defer dstConn.Close()
	defer s.connection.Close()

	s.ExchangeData(s.connection, dstConn)
}

func (s *MySocks5) ExchangeData(srcConn, dstConn net.Conn) {
	if srcConn == nil || dstConn == nil {
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		s.sslClientServerProxy(srcConn, dstConn)
	}()

	go func() {
		defer wg.Done()
		s.sslServerClientProxy(srcConn, dstConn)
	}()

	wg.Wait()
}

func (p *MySocks5) sslClientServerProxy(srcConn, dstConn net.Conn) {
	buff := make([]byte, 4096)
	for {
		n, err := srcConn.Read(buff)
		if err != nil {
			//fmt.Println("sslClientServerProxy read error:", err)
			p.closeMyChain()
			return
		}

		// 加密数据
		sslClientData := data.SocksCompress(buff[:n])

		_, err = dstConn.Write(sslClientData)
		if err != nil {
			fmt.Printf("sslClientServerProxy sendall error: %v\n", err)
			p.closeMyChain()
			return
		}
	}
}

func (p *MySocks5) sslServerClientProxy(srcConn, dstConn net.Conn) {
	buff := make([]byte, 4096)
	for {
		n, err := dstConn.Read(buff)
		if err != nil {
			//fmt.Println("sslServerClientProxy read error:", err)
			p.closeMyChain()
			return
		}

		// 解密数据
		sslServerData := data.SocksDecompress(buff[:n])
		_, err = srcConn.Write(sslServerData)
		if err != nil {
			fmt.Printf("sslServerClientProxy sendall error: %v\n", err)
			p.closeMyChain()
			return
		}
	}
}
func (p *MySocks5) closeMyChain() {
	if p.connection != nil {
		p.connection.Close()
	}
	if p.connDst != nil {
		p.connDst.Close()
	}
}

//func main() {
//	// Example usage of the MySocks5
//	listener, err := net.Listen("tcp", ":1080")
//	if err != nil {
//		fmt.Println("Failed to start server:", err)
//		os.Exit(1)
//	}
//
//	for {
//		conn, err := listener.Accept()
//		if err != nil {
//			fmt.Println("Failed to accept connection:", err)
//			continue
//		}
//		go NewMySocks5(conn, conn.RemoteAddr(), 0).Start()
//	}
//}
