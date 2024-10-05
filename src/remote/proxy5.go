package remote

import (
	"encoding/binary"
	"fmt"
	"main/utils/data"
	"net"
	"time"
)

type MySocks5 struct {
	connection net.Conn
	addr       net.Addr
}

func (s *MySocks5) Start() {
	var address string
	var remote net.Conn

	// Step 1: Client Authentication Request
	header := s.ReceiveData(2)
	if len(header) == 0 {
		s.connection.Close()
		fmt.Println("Empty Header")
		return
	}
	VER := header[0]
	NMETHODS := header[1]

	if VER != SOCKS_VERSION {
		s.connection.Close()
		fmt.Println("SOCKS version error, received version:", VER)
		return
	}

	methods := s.IsAvailable(NMETHODS)
	if !s.containsMethod(methods, 0) {
		s.connection.Close()
		return
	}

	// Step 2: Server Authentication Response
	s.SendData([]byte{SOCKS_VERSION, 0})

	// Step 3: Client Connection Request
	header = s.ReceiveData(4)
	version, cmd, _, addressType := header[0], header[1], header[2], header[3]

	if version != SOCKS_VERSION {
		fmt.Println("54 SOCKS version error")
		return
	}

	if addressType == 1 { // IPv4
		address = net.IP(s.ReceiveData(4)).String()
	} else if addressType == 3 { // Domain
		domainLength := s.ReceiveData(1)[0]
		address = string(s.ReceiveData(int(domainLength)))
	}
	portBytes := s.ReceiveData(2)
	port := binary.BigEndian.Uint16(portBytes)

	// Step 4: Server Connection Response
	if cmd == 1 { // CONNECT
		var err error
		remote, err = net.Dial("tcp", fmt.Sprintf("%s:%d", address, port))
		if err != nil {
			if s.connection != nil {
				s.connection.Close()
			}
			return
		}
		bindAddress := remote.LocalAddr().(*net.TCPAddr)
		reply := []byte{SOCKS_VERSION, 0, 0, 1}
		reply = append(reply, bindAddress.IP.To4()...)
		portBytes := make([]byte, 2)
		binary.BigEndian.PutUint16(portBytes, uint16(bindAddress.Port))
		reply = append(reply, portBytes...)
		s.SendData(reply)
	} else {
		s.connection.Close()
		fmt.Println("Unsupported command:", cmd)
		return
	}

	// Step 5: Data Exchange
	if remote != nil {
		s.ExchangeData(s.connection, remote)
		remote.Close()
	}
	s.connection.Close()
}

func (s *MySocks5) IsAvailable(n byte) []byte {
	methods := make([]byte, n)
	for i := byte(0); i < n; i++ {
		methods[i] = s.ReceiveData(1)[0]
	}
	return methods
}

func (s *MySocks5) containsMethod(methods []byte, method byte) bool {
	for _, m := range methods {
		if m == method {
			return true
		}
	}
	return false
}

func (s *MySocks5) SendData(data1 []byte) {
	// Send data over the connection
	data1 = data.SocksCompress(data1)
	s.connection.Write(data1)
}

func (s *MySocks5) ReceiveData(len int) []byte {
	buffer := make([]byte, len)
	_, err := s.connection.Read(buffer)
	if err != nil {
		fmt.Println("120： Error reading data:", err)
	}
	data.SocksDecompress(buffer)
	return buffer
}

func (s *MySocks5) ExchangeData(client net.Conn, remote net.Conn) {
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

//func main() {
//	listener, err := net.Listen("tcp", ":1080")
//	if err != nil {
//		fmt.Println("Error listening:", err)
//		os.Exit(1)
//	}
//	defer listener.Close()
//
//	for {
//		conn, err := listener.Accept()
//		if err != nil {
//			fmt.Println("Error accepting connection:", err)
//			continue
//		}
//
//		socks := &MySocks5{
//			connection: conn,
//		}
//		go socks.Start()
//	}
//}
