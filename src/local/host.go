package local

import (
	"fmt"
	"main/utils/netF"
	"net"
	"sync"
	"time"
)

type MyHost struct {
	connSrc    net.Conn
	hostname   string
	port       int
	allSrcData []byte
	sslFlag    bool
	connDst    net.Conn
}

func NewMyHost(conn net.Conn, hostname string, port int, allSrcData []byte, sslFlag bool) *MyHost {
	return &MyHost{
		connSrc:    conn,
		hostname:   hostname,
		port:       port,
		allSrcData: allSrcData,
		sslFlag:    sslFlag,
	}
}

func (h *MyHost) Start() {
	allDstData := h.getDataFromHost(h.allSrcData)
	if len(allDstData) > 0 && !h.sslFlag {
		h.sslClientServerClient(h.connSrc, h.connDst, allDstData)
	} else if h.sslFlag {
		sampleDataToClient := []byte("HTTP/1.0 200 Connection Established\r\n\r\n")
		h.sslClientServerClient(h.connSrc, h.connDst, sampleDataToClient)
	} else {
		fmt.Printf("Please check network. Cannot connect to hostname: %s\n", h.hostname)
	}
}

func (h *MyHost) getDataFromHost(rcdata []byte) []byte {
	var err error
	h.connDst, err = net.DialTimeout("tcp", fmt.Sprintf("%s:%d", h.hostname, h.port), 10*time.Second)
	if err != nil {
		//fmt.Printf("getDataFromHost - connect %s:%d : %v\n", hostname, port, err)
		return nil
	}

	if h.sslFlag {
		return nil
	}

	_, err = h.connDst.Write(rcdata)
	if err != nil {
		fmt.Printf("getDataFromHost - sendall : %v\n", err)
		netF.CloseConnection(h.connDst)
		return nil
	}

	buf := make([]byte, 4096) // Example buffer size
	n, err := h.connDst.Read(buf)
	if err != nil {
		fmt.Printf("getDataFromHost - recv : %v\n", err)
		netF.CloseConnection(h.connDst)
		return nil
	}
	return buf[:n]
}

func (h *MyHost) sslClientServerClient(srcConn net.Conn, dstConn net.Conn, allDstData []byte) {
	_, err := srcConn.Write(allDstData)
	if err != nil {
		//fmt.Printf("ssl_client_server_client - send all: %v\n", err)
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		h.sslClientServer(srcConn, dstConn)
	}()
	go func() {
		defer wg.Done()
		h.sslServerClient(srcConn, dstConn)
	}()
	wg.Wait()
}

func (h *MyHost) sslClientServer(srcConn net.Conn, dstConn net.Conn) {
	buf := make([]byte, 4096)
	for {
		n, err := srcConn.Read(buf)
		if err != nil {
			h.closeMyChain()
			return
		}
		if n > 0 {
			_, err := dstConn.Write(buf[:n])
			if err != nil {
				fmt.Printf("sslClientServer - sendall: %v\n", err)
				h.closeMyChain()
				return
			}
		} else {
			h.closeMyChain()
			return
		}
	}
}

func (h *MyHost) sslServerClient(srcConn net.Conn, dstConn net.Conn) {
	buf := make([]byte, 4096)
	for {
		n, err := dstConn.Read(buf)
		if err != nil {
			h.closeMyChain()
			return
		}
		if n > 0 {
			_, err := srcConn.Write(buf[:n])
			if err != nil {
				fmt.Printf("sslServerClient - sendall: %v\n", err)
				h.closeMyChain()
				return
			}
		} else {
			h.closeMyChain()
			return
		}
	}
}

func (h *MyHost) closeMyChain() {
	netF.CloseConnection(h.connSrc)
	netF.CloseConnection(h.connDst)
}

//func main() {
//	conn_src, _ := netF.Dial("tcp", "localhost:8080") // Example connection
//	host := NewMyHost(conn_src, "example.com", 80, []byte("GET / HTTP/1.1\r\n\r\n"), false, 8080)
//	host.Start()
//}
