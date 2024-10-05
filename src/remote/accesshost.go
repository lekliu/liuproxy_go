package remote

import (
	"bytes"
	"fmt"
	data "main/utils/data"
	"net"
	"strconv"
	"strings"
)

type ACCESS_HOST struct {
	conn net.Conn
	addr net.Addr
}

func (a *ACCESS_HOST) handler(conn net.Conn, addr net.Addr, myPort int) {
	a.conn = conn
	a.addr = addr

	if myPort == cfg.RemoteConf.PortSocks5Svr {
		socks5 := MySocks5{connection: a.conn, addr: a.addr}
		socks5.Start()
		return
	}

	if myPort == cfg.RemoteConf.PortSocks8Svr {
		socks8 := MySocks8{connection: a.conn, addr: a.addr}
		socks8.Start()
		return
	}

	allSrcData, hostname, port, sslFlag := a.getDstHostFromHeader()
	host := MyHost{
		conn: a.conn, hostname: hostname, port: port,
		allSrcData: allSrcData, sslFlag: sslFlag}
	host.Start()
}

func (a *ACCESS_HOST) getDstHostFromHeader() ([]byte, string, int, bool) {
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
		header = data.UpDecompressHeader(header)

		if len(header) > 0 {
			// 检查是否为SSL请求
			firstLine := strings.Split(string(header), "\n")[0]
			if strings.Contains(firstLine, "CONNECT") {
				hostname := strings.TrimSpace(strings.Split(firstLine, " ")[1])
				hostname = strings.TrimSpace(strings.Split(hostname, ":")[0])
				return header, hostname, 443, true
			}

			// 检查是否包含Host
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
	}
	return nil, "", 0, false
}
