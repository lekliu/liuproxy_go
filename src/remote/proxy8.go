package remote

import (
	"encoding/binary"
	"fmt"
	"main/utils/data"
	"net"
	"time"
)

type MySocks8 struct {
	connection net.Conn
	addr       net.Addr
}

func (s *MySocks8) Start() {
	var address string
	var remote net.Conn

	// 客户端认证请求
	header := s.RecieveDataCrypt(3)
	if len(header) == 0 {
		s.connection.Close()
		fmt.Println("空Header")
		return
	}

	version := header[0]
	nMethods := header[1]
	method := header[2]

	if version != SOCKS_VERSION {
		s.connection.Close()
		fmt.Println("SOCKS版本错误,传入版本为:", version)
		return
	}
	if nMethods != 1 || method != 0 {
		fmt.Println("不支持该证认方法,传入的是，nMethods:", nMethods, "，method:", method)
		s.connection.Close()
		return
	}

	// 服务端回应认证
	s.SendDataCrypt([]byte{SOCKS_VERSION, 0})

	// 客户端连接请求
	header = s.RecieveDataCrypt(4)
	version, cmd, addressType := header[0], header[1], header[3]

	if version != SOCKS_VERSION {
		s.connection.Close()
		fmt.Println("SOCKS版本错误,第2次传入版本为:", version)
		return
	}

	switch addressType {
	case 1: // IPv4
		address = net.IP(s.RecieveDataCrypt(4)).String()
	case 3: // Domain
		domainLength := s.RecieveDataCrypt(1)[0]
		address = string(s.RecieveDataCrypt(int(domainLength)))
	}

	port := int(binary.BigEndian.Uint16(s.RecieveDataCrypt(2)))

	// 服务端回应连接
	if cmd == 1 { // CONNECT
		remote, _ = net.Dial("tcp", fmt.Sprintf("%s:%d", address, port))
		bindAddress := remote.LocalAddr().(*net.TCPAddr)
		fmt.Printf("已建立连接：%s %d %s\n", address, port, bindAddress)

		reply := []byte{SOCKS_VERSION, 0, 0, 1}
		reply = append(reply, bindAddress.IP.To4()...)
		portBytes := make([]byte, 2)
		binary.BigEndian.PutUint16(portBytes, uint16(bindAddress.Port))
		reply = append(reply, portBytes...)

		s.SendData(reply)

		if reply[1] == 0 && cmd == 1 {
			s.ExchangeData(s.connection, remote)
		}
	} else if cmd == 3 { // UDP ASSOCIATE
		udpAddr, err := net.ResolveUDPAddr("udp", "0.0.0.0:0")
		if err != nil {
			fmt.Println("UDP地址解析错误:", err)
			s.connection.Close()
			return
		}
		udpSock, err := net.ListenUDP("udp", udpAddr)
		if err != nil {
			fmt.Println("UDP监听失败:", err)
			s.connection.Close()
			return
		}
		defer udpSock.Close()

		bindAddress := udpSock.LocalAddr().(*net.UDPAddr)
		fmt.Printf("UDP Associate: 已绑定UDP地址 %s\n", bindAddress)

		reply := []byte{SOCKS_VERSION, 0, 0, 1}
		reply = append(reply, bindAddress.IP.To4()...)
		portBytes := make([]byte, 2)
		binary.BigEndian.PutUint16(portBytes, uint16(bindAddress.Port))
		reply = append(reply, portBytes...)

		s.SendData(reply)

		if reply[1] == 0 && cmd == 3 {
			s.udpRelay(udpSock)
		}
	}
	s.connection.Close()
}

func (s *MySocks8) udpRelay(udpSock *net.UDPConn) {
	defer udpSock.Close()
	buffer := make([]byte, 65535)
	clientAddr := &net.UDPAddr{}
	udpSock.SetReadDeadline(time.Now().Add(60 * time.Second))

	for {
		n, addr, err := udpSock.ReadFromUDP(buffer)
		if err != nil {
			fmt.Println("UDP Relay 读取失败或超时:", err)
			break
		}

		// 解析请求
		rsv := binary.BigEndian.Uint16(buffer[0:2])
		if rsv == 0 { // UDP客户端发来的UDP数据包
			clientAddr = addr
			addressType := buffer[3]

			var targetAddress string
			var targetPort int
			var payload []byte

			switch addressType {
			case 1: // IPv4
				targetAddress = net.IP(buffer[4:8]).String()
				targetPort = int(binary.BigEndian.Uint16(buffer[8:10]))
				payload = buffer[10:n]
			case 3: // Domain name
				domainLength := int(buffer[4])
				targetAddress = string(buffer[5 : 5+domainLength])
				targetPort = int(binary.BigEndian.Uint16(buffer[5+domainLength : 7+domainLength]))
				payload = buffer[7+domainLength : n]
			}

			// 转发数据到目标地址
			targetAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", targetAddress, targetPort))
			if err != nil {
				fmt.Println("目标地址解析错误:", err)
				continue
			}

			_, err = udpSock.WriteToUDP(payload, targetAddr)
			if err != nil {
				fmt.Println("转发到目标地址失败:", err)
				continue
			}
		} else { // 远程UDP服务器返回的数据
			responseHeader := make([]byte, 10)
			binary.BigEndian.PutUint16(responseHeader[0:2], 0)
			responseHeader[2] = 0
			responseHeader[3] = 1
			copy(responseHeader[4:], addr.IP.To4())
			binary.BigEndian.PutUint16(responseHeader[8:], uint16(addr.Port))

			udpSock.WriteToUDP(append(responseHeader, buffer[0:n]...), clientAddr)
		}
	}
}

func (s *MySocks8) RecieveDataCrypt(length int) []byte {
	buf := make([]byte, length)
	s.connection.Read(buf)
	return buf
}

func (s *MySocks8) SendDataCrypt(data []byte) {
	s.connection.Write(data)
}

func (s *MySocks8) RecieveData(length int) []byte {
	buf := make([]byte, length)
	s.connection.Read(buf)
	return buf
}

func (s *MySocks8) SendData(data []byte) {
	s.connection.Write(data)
}

func (s *MySocks8) ExchangeData(client net.Conn, remote net.Conn) {
	bufferSize := 2048
	done := make(chan bool)

	// 从client到remote的数据传输
	go func() {
		buffer := make([]byte, bufferSize)
		for {
			client.SetReadDeadline(time.Now().Add(30 * time.Second))
			n, err := client.Read(buffer)
			if err != nil {
				done <- true
				return
			}

			// 解密数据
			decryptedData := data.SocksDecompress(buffer[:n])

			_, err = remote.Write(decryptedData)
			if err != nil {
				done <- true
				return
			}
		}
	}()

	// 从remote到client的数据传输
	go func() {
		buffer := make([]byte, bufferSize)
		for {
			remote.SetReadDeadline(time.Now().Add(30 * time.Second))
			n, err := remote.Read(buffer)
			if err != nil {
				done <- true
				return
			}

			// 加密数据
			encryptedData := data.SocksCompress(buffer[:n])

			_, err = client.Write(encryptedData)
			if err != nil {
				done <- true
				return
			}
		}
	}()

	// 等待其中一个 goroutine 完成
	<-done
}
