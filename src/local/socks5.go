package local

import (
	"fmt"
	"main/utils/data_crypt"
	"main/utils/netF"
	"net"
	"sync"
)

type MySocks5 struct {
	connSrc    net.Conn
	addr       net.Addr
	remoteIP   string
	remotePort int
	connDst    net.Conn
}

func NewMySocks5(conn net.Conn, addr net.Addr, remoteIP string, remotePort int) *MySocks5 {
	return &MySocks5{
		connSrc:    conn,
		addr:       addr,
		remoteIP:   remoteIP,
		remotePort: remotePort,
	}
}

func (s *MySocks5) Start() {
	dstConn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", s.remoteIP, s.remotePort))
	if err != nil {
		fmt.Println("Connection error:", err)
		return
	}
	s.connDst = dstConn

	header := s.ReceiveData(3)
	if len(header) == 0 {
		netF.CloseConnWithInfo(s.connSrc, "空Header")
		return
	}
	header = data_crypt.UpCompressHeader(header, cfg.CommonConf.Crypt)
	_, err = s.connDst.Write(header)
	if err != nil {
		fmt.Printf("socks5.go 44 error: %v\n", err)
		s.closeMyChain()
		return
	}

	defer s.closeMyChain()
	s.ExchangeData()
}

func (s *MySocks5) ExchangeData() {
	if s.connSrc == nil || s.connDst == nil {
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		s.sslClientServerProxy()
	}()

	go func() {
		defer wg.Done()
		s.sslServerClientProxy()
	}()

	wg.Wait()
}

func (s *MySocks5) sslClientServerProxy() {
	buff := make([]byte, 4096)
	for {
		n, err := s.connSrc.Read(buff)
		if err != nil {
			//fmt.Println("sslClientServerProxy read error:", err)
			s.closeMyChain()
			return
		}

		// 加密数据
		sslClientData := data_crypt.SocksCompress(buff[:n], cfg.CommonConf.Crypt)

		_, err = s.connDst.Write(sslClientData)
		if err != nil {
			fmt.Printf("sslClientServerProxy sendall error: %v\n", err)
			s.closeMyChain()
			return
		}
	}
}

func (s *MySocks5) sslServerClientProxy() {
	buff := make([]byte, 4096)
	for {
		n, err := s.connDst.Read(buff)
		if err != nil {
			//fmt.Println("sslServerClientProxy read error:", err)
			s.closeMyChain()
			return
		}

		// 解密数据
		sslServerData := data_crypt.SocksDecompress(buff[:n], cfg.CommonConf.Crypt)
		_, err = s.connSrc.Write(sslServerData)
		if err != nil {
			fmt.Printf("sslServerClientProxy sendall error: %v\n", err)
			s.closeMyChain()
			return
		}
	}
}
func (s *MySocks5) closeMyChain() {
	netF.CloseConnection(s.connSrc)
	netF.CloseConnection(s.connDst)
}

func (s *MySocks5) ReceiveData(length int) []byte {
	buf := make([]byte, length)
	_, err := s.connSrc.Read(buf)
	if err != nil {
		return nil
	}
	return buf
}

//func main() {
//	// Example usage of the MySocks5
//	listener, err := netF.Listen("tcp", ":1080")
//	if err != nil {
//		fmt.Println("Failed to start server:", err)
//		os.Exit(1)
//	}
//
//	for {
//		conn_src, err := listener.Accept()
//		if err != nil {
//			fmt.Println("Failed to accept connection:", err)
//			continue
//		}
//		go NewMySocks5(conn_src, conn_src.RemoteAddr(), 0).Start()
//	}
//}
