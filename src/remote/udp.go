package remote

import (
	"encoding/binary"
	"fmt"
	"log"
	"main/utils/data_crypt"
	"net"
	"time"
)

type MyUdp struct {
	connSrc net.Conn
}

func NewMyUdp(conn net.Conn) *MyUdp {
	return &MyUdp{
		connSrc: conn,
	}
}

func (u *MyUdp) Start() {
	udpAddr, err := net.ResolveUDPAddr("udp", "0.0.0.0:0")
	if err != nil {
		u.handleError("UDP地址解析错误", err)
		return
	}

	udpSock, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		u.handleError("UDP监听失败", err)
		return
	}
	defer func() {
		if err := udpSock.Close(); err != nil {
			fmt.Println("关闭UDP连接时出错:", err)
		}
	}()

	bindAddress := udpSock.LocalAddr().(*net.UDPAddr)
	// fmt.Printf("UDP Associate: 已绑定UDP地址 %s\n", bindAddress)

	// 构建回复报文
	reply := buildReply(bindAddress)
	u.SendData(reply)

	if reply[1] == 0 {
		u.udpRelay(udpSock)
	}
}

func (u *MyUdp) udpRelay(udpSock *net.UDPConn) {
	buffer := make([]byte, 65535)
	clientAddr := &net.UDPAddr{}

	// 设置超时
	err := udpSock.SetReadDeadline(time.Now().Add(Timeout))
	if err != nil {
		u.handleError("设置超时失败", err)
		return
	}

	for {
		n, addr, err := udpSock.ReadFromUDP(buffer)
		if err != nil {
			u.handleError("UDP Relay 读取失败或超时", err)
			break
		}

		rsv := binary.BigEndian.Uint16(buffer[0:2])
		if rsv == 0 {
			// 处理来自客户端的数据
			clientAddr = addr
			if err := u.handleClientData(udpSock, buffer[:n]); err != nil {
				u.handleError("处理客户端数据失败", err)
			}
		} else {
			// 处理来自远程服务器的数据
			if err := u.handleServerData(udpSock, buffer[:n], addr, clientAddr); err != nil {
				u.handleError("处理服务器数据失败", err)
			}
		}
	}
}

func (u *MyUdp) handleClientData(udpSock *net.UDPConn, data []byte) error {
	addressType := data[3]
	var targetAddress string
	var targetPort int
	var payload []byte

	switch addressType {
	case 1: // IPv4
		targetAddress = net.IP(data[4:8]).String()
		targetPort = int(binary.BigEndian.Uint16(data[8:10]))
		payload = data[10:]
	case 3: // 域名
		domainLength := int(data[4])
		targetAddress = string(data[5 : 5+domainLength])
		targetPort = int(binary.BigEndian.Uint16(data[5+domainLength : 7+domainLength]))
		payload = data[7+domainLength:]
	default:
		return fmt.Errorf("不支持的地址类型: %d", addressType)
	}

	targetAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", targetAddress, targetPort))
	if err != nil {
		return fmt.Errorf("目标地址解析错误: %w", err)
	}

	_, err = udpSock.WriteToUDP(payload, targetAddr)
	return err
}

func (u *MyUdp) handleServerData(udpSock *net.UDPConn, data []byte, addr *net.UDPAddr, clientAddr *net.UDPAddr) error {
	responseHeader := make([]byte, 10)
	binary.BigEndian.PutUint16(responseHeader[0:2], 0)
	responseHeader[2] = 0
	responseHeader[3] = 1
	copy(responseHeader[4:], addr.IP.To4())
	binary.BigEndian.PutUint16(responseHeader[8:], uint16(addr.Port))

	// fmt.Println(responseHeader)
	// fmt.Println(data)
	_, err := udpSock.WriteToUDP(append(responseHeader, data...), clientAddr)
	return err
}

func (u *MyUdp) SendData(data []byte) {
	data = data_crypt.SocksCompress(data, cfg.CommonConf.Crypt)
	_, err := u.connSrc.Write(data)
	if err != nil {
		u.handleError("发送数据失败", err)
	}
}

func (u *MyUdp) handleError(msg string, err error) {
	log.Printf("%s: %v\n", msg, err)
	if u.connSrc != nil {
		_ = u.connSrc.Close()
	}
}

func buildReply(bindAddress *net.UDPAddr) []byte {
	reply := []byte{SocksVersion, 0, 0, 1}
	reply = append(reply, bindAddress.IP.To4()...)
	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, uint16(bindAddress.Port))
	reply = append(reply, portBytes...)
	return reply
}
