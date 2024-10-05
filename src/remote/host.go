package remote

import (
	"fmt"
	data "main/utils/data"
	"net"
	"sync"
)

type MyHost struct {
	conn       net.Conn
	hostname   string
	port       int
	allSrcData []byte
	sslFlag    bool
	connDst    net.Conn
	wg         sync.WaitGroup
}

func (h *MyHost) Start() {
	allDstData := h.getDataFromHost(h.hostname, h.port, h.allSrcData, h.sslFlag)

	if allDstData != nil {
		h.sslClientServerClient(h.conn, h.connDst, allDstData)
	} else {
		h.conn.Close()
	}
}

func (h *MyHost) getDataFromHost(host string, port int, sdata []byte, sslFlag bool) []byte {
	var err error
	h.connDst, err = net.Dial("tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		if h.connDst != nil {
			h.connDst.Close()
		}
		return nil
	}

	if sslFlag {
		return []byte("HTTP/1.0 200 Connection Established\r\n\r\n")
	}

	_, err = h.connDst.Write(sdata)
	if err != nil {
		h.connDst.Close()
		return nil
	}

	rcData := make([]byte, cfg.CommonConf.BufferSize)
	n, err := h.connDst.Read(rcData)
	if err != nil {
		h.connDst.Close()
		return nil
	}

	return rcData[:n]
}

func (h *MyHost) sslClientServerClient(srcConn, dstConn net.Conn, allDstData []byte) {
	h.allSrcData = data.DownCompress(allDstData) // Assuming DownCompress is a method in util
	if !h.checkMyChain() {
		return
	}

	_, err := srcConn.Write(h.allSrcData)
	if err != nil {
		h.closeMyChain()
		return
	}

	h.wg.Add(2)
	go h.sslClientServer(srcConn, dstConn)
	go h.sslServerClient(srcConn, dstConn)
}

func (h *MyHost) sslClientServer(srcConn, dstConn net.Conn) {
	defer h.wg.Done()
	for {
		if !h.checkMyChain() {
			return
		}

		sslClientData := make([]byte, cfg.CommonConf.BufferSize)
		n, err := srcConn.Read(sslClientData)
		if err != nil {
			h.closeMyChain()
			return
		}

		if n > 0 {
			sslClientData = data.UpDecompress(sslClientData[:n]) // Assuming UpDecompress is a method in util
			if !h.checkMyChain() {
				return
			}
			_, err := dstConn.Write(sslClientData)
			if err != nil {
				h.closeMyChain()
				return
			}
		} else {
			h.closeMyChain()
			return
		}
	}
}

func (h *MyHost) sslServerClient(srcConn, dstConn net.Conn) {
	defer h.wg.Done()
	for {
		if !h.checkMyChain() {
			return
		}

		sslServerData := make([]byte, cfg.CommonConf.BufferSize)
		n, err := dstConn.Read(sslServerData)
		if err != nil {
			h.closeMyChain()
			return
		}

		if n > 0 {
			sslServerData = data.DownCompress(sslServerData[:n]) // Assuming DownCompress is a method in util
			if !h.checkMyChain() {
				return
			}
			_, err := srcConn.Write(sslServerData)
			if err != nil {
				h.closeMyChain()
				return
			}
		} else {
			h.closeMyChain()
			return
		}
	}
}

func (h *MyHost) checkMyChain() bool {
	if h.conn != nil && h.connDst != nil {
		return true
	}
	h.closeMyChain()
	return false
}

func (h *MyHost) closeMyChain() {
	if h.conn != nil {
		h.conn.Close()
	}
	if h.connDst != nil {
		h.connDst.Close()
	}
}
