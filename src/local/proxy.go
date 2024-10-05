package local

import (
	"fmt"
	"io"
	"main/utils/data"
	"net"
	"strconv"
	"sync"
)

type MyProxy struct {
	conn       net.Conn
	hostname   string
	port       int
	allSrcData []byte
	sslFlag    bool
	remoteIP   string
	remotePort int
	connDst    net.Conn
}

func NewMyProxy(conn net.Conn, hostname string, port int, allSrcData []byte, sslFlag bool, remoteID int) *MyProxy {
	return &MyProxy{
		conn:       conn,
		hostname:   hostname,
		port:       port,
		allSrcData: allSrcData,
		sslFlag:    sslFlag,
		remoteIP:   GetRemoteIP(remoteID),   // 假设有获取远程 IP 的函数
		remotePort: GetRemotePort(remoteID), // 假设有获取远程端口的函数
	}
}

func (p *MyProxy) Start() {
	allDstData := p.getDataFromProxy(p.allSrcData)
	if allDstData != nil {
		p.sslClientServerClientProxy(p.conn, p.connDst, allDstData)
	} else {
		p.conn.Close()
	}
}

func (p *MyProxy) getDataFromProxy(sdata []byte) []byte {
	var err error
	p.connDst, err = net.Dial("tcp", fmt.Sprintf("%s:%d", p.remoteIP, p.remotePort))
	if err != nil {
		//fmt.Printf("getDataFromProxy: cannot connect host: %s\n", p.hostname)
		return nil
	}

	// 加密数据
	sdata = data.UpCompressHeader(sdata)

	_, err = p.connDst.Write(sdata)
	if err != nil {
		fmt.Printf("getDataFromProxy sendall: %v\n", err)
		p.connDst.Close()
		return nil
	}

	buff := make([]byte, 4096)
	n, err := p.connDst.Read(buff)
	if err != nil && err != io.EOF {
		fmt.Printf("getDataFromProxy recv: %v\n", err)
		p.connDst.Close()
		return nil
	}

	// 解密数据 (这里假设 downDecompress 是解密函数)
	return data.DownDecompress(buff[:n])
}

func (p *MyProxy) sslClientServerClientProxy(srcConn, dstConn net.Conn, allDstData []byte) {
	_, err := srcConn.Write(allDstData)
	if err != nil {
		fmt.Println("cannot send data to SSL client:", err)
		return
	}
	if srcConn == nil || dstConn == nil {
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
			//fmt.Println("sslClientServerProxy read error:", err)
			p.closeMyChain()
			return
		}

		// 加密数据
		sslClientData := data.UpCompress(buff[:n])

		_, err = dstConn.Write(sslClientData)
		if err != nil {
			fmt.Printf("sslClientServerProxy sendall error: %v\n", err)
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
			//fmt.Println("sslServerClientProxy read error:", err)
			p.closeMyChain()
			return
		}

		// 解密数据
		sslServerData := data.DownDecompress(buff[:n])

		_, err = srcConn.Write(sslServerData)
		if err != nil {
			fmt.Printf("sslServerClientProxy sendall error: %v\n", err)
			p.closeMyChain()
			return
		}
	}
}

func (p *MyProxy) checkMyChain() bool {
	return p.conn != nil && p.connDst != nil
}

func (p *MyProxy) closeMyChain() {
	if p.conn != nil {
		p.conn.Close()
	}
	if p.connDst != nil {
		p.connDst.Close()
	}
}

// 模拟获取远程 IP 和端口的函数
func GetRemoteIP(remoteID int) string {
	// 返回模拟 IP
	ip := cfg.LocalConf.RemoteIPs[remoteID][0]
	return ip
}

func GetRemotePort(remoteID int) int {
	// 返回端口
	port := cfg.LocalConf.RemoteIPs[remoteID][1]
	iPort, err := strconv.Atoi(port)
	if err != nil {
		return 0
	}
	return iPort
}

func GetRemotePort2(remoteID int) int {
	// 返回端口
	port := cfg.LocalConf.RemoteIPs[remoteID][2]
	iPort, err := strconv.Atoi(port)
	if err != nil {
		return 0
	}
	return iPort
}
