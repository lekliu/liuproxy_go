package local

import (
	"bytes"
	"fmt"
	"io"
	"main/geo"
	"main/utils/data_crypt"
	"net"
	"strconv"
	"strings"
	"sync"
)

type MyProxy struct {
	connSrc    net.Conn
	remoteIP   string
	remotePort int
	hostname   string
	port       int
	connDst    net.Conn
}

func NewMyProxy(conn net.Conn, remoteIP string, remotePort int) *MyProxy {
	return &MyProxy{
		connSrc:    conn,
		remoteIP:   remoteIP,
		remotePort: remotePort,
	}
}

func (p *MyProxy) Start() {
	// 获取目标主机信息
	allSrcData, hostname, port, sslFlag := p.getDstHostFromHeader()
	p.hostname = hostname
	p.port = port

	if !strings.Contains(hostname, ".") {
		// fmt.Printf("ERR url: %s\n", hostname)
		err := p.connSrc.Close()
		if err != nil {
			return
		}
		return
	}

	opt := geo.GeoIP(hostname)
	switch opt {
	case 0:
		// fmt.Printf("%s : 0断开\n", hostname)
		err := p.connSrc.Close()
		if err != nil {
			return
		}
		return
	case 2:
		// 启动代理
		p.getDate(allSrcData)
		return
	default:
		// fmt.Printf("%s : 1直连\n", hostname)
		go func() {
			host := NewMyHost(p.connSrc, hostname, port, allSrcData, sslFlag)
			host.Start()
		}()
	}
}

func (p *MyProxy) getDate(allSrcData []byte) {
	allDstData := p.getDataFromProxy(allSrcData)
	if allDstData != nil {
		p.sslClientServerClientProxy(p.connSrc, p.connDst, allDstData)
	} else {
		err := p.connSrc.Close()
		if err != nil {
			return
		}
	}
}

func (p *MyProxy) getDataFromProxy(srcData []byte) []byte {
	var err error
	p.connDst, err = net.Dial("tcp", fmt.Sprintf("%s:%d", p.remoteIP, p.remotePort))
	if err != nil {
		// fmt.Printf("getDataFromProxy: cannot connect host: %s\n", p.hostname)
		return nil
	}

	// 加密数据
	srcData = data_crypt.UpCompressHeader(srcData, cfg.CommonConf.Crypt)

	_, err = p.connDst.Write(srcData)
	if err != nil {
		// fmt.Printf("getDataFromProxy send all: %v\n", err)
		err := p.connDst.Close()
		if err != nil {
			return nil
		}
		return nil
	}

	buff := make([]byte, 4096)
	n, err := p.connDst.Read(buff)
	if err != nil && err != io.EOF {
		// fmt.Printf("getDataFromProxy receive: %v\n", err)
		err := p.connDst.Close()
		if err != nil {
			return nil
		}
		return nil
	}

	// 解密数据 (这里假设 downDecompress 是解密函数)
	return data_crypt.DownDecompress(buff[:n], cfg.CommonConf.Crypt)
}

func (p *MyProxy) sslClientServerClientProxy(srcConn, dstConn net.Conn, allDstData []byte) {
	_, err := srcConn.Write(allDstData)
	if err != nil {
		// fmt.Println("cannot send data_crypt to SSL client:", err)
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		p.sslClientServerProxy(srcConn, dstConn)
	}()

	go func() {
		defer wg.Done()
		p.sslServerClientProxy(srcConn, dstConn)
	}()

	wg.Wait()
}

func (p *MyProxy) sslClientServerProxy(srcConn, dstConn net.Conn) {
	buff := make([]byte, 4096)
	for {
		n, err := srcConn.Read(buff)
		if err != nil {
			// fmt.Println("sslClientServerProxy read error:", err)
			p.closeMyChain()
			return
		}

		// 加密数据
		sslClientData := data_crypt.UpCompress(buff[:n], cfg.CommonConf.Crypt)

		_, err = dstConn.Write(sslClientData)
		if err != nil {
			// fmt.Printf("sslClientServerProxy send all error: %v\n", err)
			p.closeMyChain()
			return
		}
	}
}

func (p *MyProxy) sslServerClientProxy(srcConn, dstConn net.Conn) {
	buff := make([]byte, 4096)
	for {
		n, err := dstConn.Read(buff)
		if err != nil {
			// fmt.Println("sslServerClientProxy read error:", err)
			p.closeMyChain()
			return
		}

		// 解密数据
		sslServerData := data_crypt.DownDecompress(buff[:n], cfg.CommonConf.Crypt)

		_, err = srcConn.Write(sslServerData)
		if err != nil {
			// fmt.Printf("sslServerClientProxy send all error: %v\n", err)
			p.closeMyChain()
			return
		}
	}
}

func (p *MyProxy) checkMyChain() bool {
	return p.connSrc != nil && p.connDst != nil
}

func (p *MyProxy) closeMyChain() {
	err := p.connSrc.Close()
	if err != nil {
	}
	err = p.connDst.Close()
	if err != nil {
	}
}

// getDstHostFromHeader 方法，用于解析请求头获取目标主机信息
func (p *MyProxy) getDstHostFromHeader() ([]byte, string, int, bool) {
	var header []byte
	//sslFlag := false
	for {
		// 读取头部数据
		line := make([]byte, cfg.CommonConf.BufferSize)
		n, err := p.connSrc.Read(line)
		if err == io.EOF {
			// fmt.Printf("Error reading header2: %v\n", err)
			return nil, "", 0, false
		}
		if err != nil {
			// fmt.Printf("Error reading header: %v\n", err)
			return nil, "", 0, false
		}

		if n <= 0 {
			break
		}

		line = line[:n]

		header = append(header, line...)
		firstLine := strings.Split(string(header), "\n")[0]
		// 检查是否是SSL连接
		if strings.Contains(firstLine, "CONNECT") {
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
