package local

import (
	"bytes"
	"fmt"
	"main/geo"
	"net"
	"strconv"
	"strings"
)

// AccessToHost 结构体
type AccessToHost struct {
	conn net.Conn
	addr net.Addr
}

// Handler 方法
func (a *AccessToHost) Handler(conn net.Conn, addr net.Addr, myport, remoteid int) {
	a.conn = conn
	a.addr = addr

	if myport == cfg.LocalConf.PortSocks5 {
		// 启动 Socks5 代理
		go startSocks5(conn, addr, remoteid)
		return
	}

	// 获取目标主机信息
	allSrcData, hostname, port, sslFlag := a.getDstHostFromHeader(conn)
	if !strings.Contains(hostname, ".") {
		conn.Close()
		fmt.Printf("ERR url: %s\n", hostname)
		return
	}

	opt := geo.GeoIP(hostname)
	switch opt {
	case 0:
		conn.Close()
		fmt.Printf("%s : 0断开\n", hostname)
	case 2:
		// 启动代理
		go startProxy(conn, hostname, port, allSrcData, sslFlag, remoteid)
	default:
		// fmt.Printf("%s : 1直连\n", hostname)
		go startHost(conn, hostname, port, allSrcData, sslFlag, myport)
	}
}

// getDstHostFromHeader 方法，用于解析请求头获取目标主机信息
func (a *AccessToHost) getDstHostFromHeader(connSock net.Conn) ([]byte, string, int, bool) {
	var header []byte
	//sslFlag := false
	for {
		// 读取头部数据
		line := make([]byte, cfg.CommonConf.BufferSize)
		n, err := a.conn.Read(line)

		if err != nil {
			fmt.Printf("Error reading header: %v\n", err)
			return nil, "", 0, false
		}

		if n <= 0 {
			break
		}

		line = line[:n]

		header = append(header, line...)
		firstLine := strings.Split(string(header), "\n")[0]
		// 检查是否是SSL连接
		if strings.Contains(string(firstLine), "CONNECT") {
			hostname := strings.TrimSpace(strings.Split(firstLine, " ")[1])
			hostname = strings.TrimSpace(strings.Split(hostname, ":")[0])
			return header, hostname, 443, true
		}

		// 检查Host字段
		hostIndex := bytes.Index(header, []byte("Host:"))
		if hostIndex > -1 {
			hostLine := header[hostIndex:]
			endOfLine := bytes.Index(hostLine, []byte("\n"))
			host := string(hostLine[5:endOfLine])
			host = strings.TrimSpace(host)

			if strings.Contains(host, ":") {
				parts := strings.Split(host, ":")
				port, _ := strconv.Atoi(parts[1])
				return header, parts[0], port, false
			}
			return header, host, 80, false
		}
	}
	return nil, "", 0, false
}

// 模拟 MySocks5、MyProxy 和 MyHost 的函数
func startSocks5(conn net.Conn, addr net.Addr, remoteid int) {
	socks5 := NewMySocks5(conn, addr, remoteid)
	socks5.Start()
}

func startProxy(conn net.Conn, hostname string, port int, data []byte, sslFlag bool, remoteid int) {
	proxy := NewMyProxy(conn, hostname, port, data, sslFlag, remoteid)
	proxy.Start()
}

func startHost(conn net.Conn, hostname string, port int, data []byte, sslFlag bool, myport int) {
	host := NewMyHost(conn, hostname, port, data, sslFlag, myport)
	host.Start()
}
