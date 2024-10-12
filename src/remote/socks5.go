package remote

import (
	"encoding/binary"
	"fmt"
	"main/utils/data_crypt"
	"main/utils/netF"
	"net"
	"sync"
)

type MySocks5 struct {
	connSrc net.Conn
	connDst net.Conn
}

func NewMySocks5(conn net.Conn) *MySocks5 {
	return &MySocks5{
		connSrc: conn,
	}
}

func (sock *MySocks5) Start() {
	// 客户端认证请求
	headerlen := len(data_crypt.Header)
	header := sock.ReceiveDataCrypt(3 + headerlen)
	if len(header) < headerlen+3 {
		netF.CloseConnWithInfo(sock.connSrc, "空Header")
		return
	}
	header = header[headerlen:]
	version := header[0]
	nMethods := header[1]
	method := header[2]

	if version != SocksVersion {
		netF.CloseConnWithInfo(sock.connSrc, "SOCKS版本错误,传入版本为:%d", version)
		return
	}
	if nMethods != 1 || method != 0 {
		netF.CloseConnWithInfo(sock.connSrc, "不支持该证认方法,传入的是，nMethods:", nMethods, "，method:", method)
		return
	}

	// 服务端回应认证
	sock.SendDataCrypt([]byte{SocksVersion, 0})

	// 客户端连接请求
	header = sock.ReceiveDataCrypt(4)
	version, cmd, addressType := header[0], header[1], header[3]

	if version != SocksVersion {
		netF.CloseConnWithInfo(sock.connSrc, "SOCKS版本错误,第2次传入版本为:", version)
		return
	}

	var address string
	switch addressType {
	case 1: // IPv4
		address = net.IP(sock.ReceiveDataCrypt(4)).String()
	case 3: // Domain
		domainLength := sock.ReceiveDataCrypt(1)[0]
		address = string(sock.ReceiveDataCrypt(int(domainLength)))
	}

	port := int(binary.BigEndian.Uint16(sock.ReceiveDataCrypt(2)))

	// 服务端回应连接
	if cmd == 1 { // CONNECT
		connection, err := net.Dial("tcp", fmt.Sprintf("%s:%d", address, port))
		sock.connDst = connection
		if err != nil {
			// netF.CloseConnection(s.connSrc, "连接失败:", err)
			netF.CloseConnection(sock.connSrc)
			return
		}
		bindAddress := sock.connDst.LocalAddr().(*net.TCPAddr)
		// fmt.Printf("已建立连接：%s %d %s\n", address, port, bindAddress)

		reply := []byte{SocksVersion, 0, 0, 1}
		reply = append(reply, bindAddress.IP.To4()...)
		portBytes := make([]byte, 2)
		binary.BigEndian.PutUint16(portBytes, uint16(bindAddress.Port))
		reply = append(reply, portBytes...)

		// fmt.Println(reply)
		flag := reply[1]
		sock.SendDataCrypt(reply)

		if flag == 0 {
			sock.ExchangeData(sock.connSrc, sock.connDst)
		}
	} else if cmd == 3 { // UDP ASSOCIATE
		udp := NewMyUdp(sock.connSrc)
		udp.Start()
		return
	}
	err := sock.connSrc.Close()
	if err != nil {
		return
	}
}

func (sock *MySocks5) ReceiveDataCrypt(length int) []byte {
	buf := make([]byte, length)
	_, err := sock.connSrc.Read(buf)
	if err != nil {
		return nil
	}
	buf = data_crypt.SocksDecompress(buf, cfg.CommonConf.Crypt)
	return buf
}

func (sock *MySocks5) SendDataCrypt(data []byte) {
	data = data_crypt.SocksCompress(data, cfg.CommonConf.Crypt)
	_, err := sock.connSrc.Write(data)
	if err != nil {
		return
	}
}

func (sock *MySocks5) ExchangeData(srcConn, dstConn net.Conn) {
	if srcConn == nil || dstConn == nil {

		return
	}
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		sock.sslClientServerProxy(srcConn, dstConn)
	}()

	go func() {
		defer wg.Done()
		sock.sslServerClientProxy(srcConn, dstConn)
	}()

	wg.Wait()
}

func (sock *MySocks5) sslClientServerProxy(srcConn, dstConn net.Conn) {
	buff := make([]byte, 4096)
	for {
		n, err := srcConn.Read(buff)
		if err != nil {
			//fmt.Println("sslClientServerProxy read error:", err)
			sock.closeMyChain()
			return
		}

		// 加密数据
		sslClientData := data_crypt.SocksDecompress(buff[:n], cfg.CommonConf.Crypt)

		_, err = dstConn.Write(sslClientData)
		if err != nil {
			fmt.Printf("sslClientServerProxy sendall error: %v\n", err)
			sock.closeMyChain()
			return
		}
	}
}

func (sock *MySocks5) sslServerClientProxy(srcConn, dstConn net.Conn) {
	buff := make([]byte, 4096)
	for {
		n, err := dstConn.Read(buff)
		if err != nil {
			//fmt.Println("sslServerClientProxy read error:", err)
			sock.closeMyChain()
			return
		}

		// 解密数据
		sslServerData := data_crypt.SocksCompress(buff[:n], cfg.CommonConf.Crypt)
		_, err = srcConn.Write(sslServerData)
		if err != nil {
			// fmt.Printf("sslServerClientProxy send all error: %v\n", err)
			sock.closeMyChain()
			return
		}
	}
}
func (sock *MySocks5) closeMyChain() {
	err := sock.connSrc.Close()
	if err != nil {
		return
	}
	err = sock.connDst.Close()
	if err != nil {
		return
	}
}
