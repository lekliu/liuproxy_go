package remote

import (
	"encoding/binary"
	"fmt"
	"net"
	"time"
)

type MyUdp struct {
	connSrc net.Conn
	addr    net.Addr
}

func NewMyUdp(conn net.Conn) *MyUdp {
	return &MyUdp{
		connSrc: conn,
	}
}

func (u *MyUdp) Start() {
	udpAddr, err := net.ResolveUDPAddr("udp", "0.0.0.0:0")
	if err != nil {
		fmt.Println("UDP地址解析错误:", err)
		u.connSrc.Close()
		return
	}
	udpSock, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		fmt.Println("UDP监听失败:", err)
		u.connSrc.Close()
		return
	}
	defer udpSock.Close()

	bindAddress := udpSock.LocalAddr().(*net.UDPAddr)
	fmt.Printf("UDP Associate: 已绑定UDP地址 %s\n", bindAddress)

	reply := []byte{SocksVersion, 0, 0, 1}
	reply = append(reply, bindAddress.IP.To4()...)
	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, uint16(bindAddress.Port))
	reply = append(reply, portBytes...)

	u.SendData(reply)

	if reply[1] == 0 {
		u.udpRelay(udpSock)
	}
	err = u.connSrc.Close()
	if err != nil {
		return
	}
}

func (u *MyUdp) udpRelay(udpSock *net.UDPConn) {
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

func (u *MyUdp) SendData(data []byte) {
	_, err := u.connSrc.Write(data)
	if err != nil {
		return
	}
}
