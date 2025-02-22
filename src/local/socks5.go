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
	flag       int
	connDst    net.Conn
}

func NewMySocks5(conn net.Conn, addr net.Addr, remoteIP string, remotePort int, flag int) *MySocks5 {
	return &MySocks5{
		connSrc:    conn,
		addr:       addr,
		remoteIP:   remoteIP,
		flag:       flag,
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
	var header []byte
	if s.flag == 1 {
		header = s.ReceiveData(3)
	} else {
		header = s.ReceiveDataCrypt(3)
	}
	if len(header) == 0 {
		netF.CloseConnWithInfo(s.connSrc, "空Header")
		return
	}
	if s.flag == 1 {
		header = data_crypt.UpCompressHeader(header, cfg.CommonConf.Crypt)
	}
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
		if s.flag == 1 {
			s.sslProxy_none_crypt(s.connSrc, s.connDst)
		} else {
			s.sslProxy_crypt_none(s.connSrc, s.connDst)
		}
	}()

	go func() {
		defer wg.Done()
		if s.flag == 1 {
			s.sslProxy_crypt_none(s.connDst, s.connSrc)
		} else {
			s.sslProxy_none_crypt(s.connDst, s.connSrc)
		}
	}()

	wg.Wait()
}

func (s *MySocks5) sslProxy_none_crypt(conn1, conn2 net.Conn) {
	buff := make([]byte, 4096)
	for {
		n, err := conn1.Read(buff)
		if err != nil {
			//fmt.Println("sslClientServerProxy read error:", err)
			s.closeMyChain()
			return
		}

		// 加密数据
		sslClientData := data_crypt.SocksCompress(buff[:n], cfg.CommonConf.Crypt)
		_, err = conn2.Write(sslClientData)
		if err != nil {
			fmt.Printf("sslClientServerProxy sendall error: %v\n", err)
			s.closeMyChain()
			return
		}
	}
}

func (s *MySocks5) sslProxy_crypt_none(conn1, conn2 net.Conn) {
	buff := make([]byte, 4096)
	for {
		n, err := conn1.Read(buff)
		if err != nil {
			//fmt.Println("sslServerClientProxy read error:", err)
			s.closeMyChain()
			return
		}

		// 解密数据
		sslServerData := data_crypt.SocksDecompress(buff[:n], cfg.CommonConf.Crypt)
		_, err = conn2.Write(sslServerData)
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

func (s *MySocks5) ReceiveDataCrypt(length int) []byte {
	buf := make([]byte, length)
	_, err := s.connSrc.Read(buf)
	if err != nil {
		return nil
	}
	buf = data_crypt.SocksDecompress(buf, cfg.CommonConf.Crypt)
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
