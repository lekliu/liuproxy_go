package remote

import (
	"bytes"
	"fmt"
	data "main/utils/data_crypt"
	"net"
	"strconv"
	"strings"
	"sync"
)

type MyHost struct {
	connSrc  net.Conn
	hostname string
	port     int
	connDst  net.Conn
	wg       sync.WaitGroup
}

func NewMyHost(conn net.Conn) *MyHost {
	return &MyHost{
		connSrc: conn,
	}
}

func (h *MyHost) Start() {
	allSrcData, hostname, port, sslFlag := h.getDstHostFromHeader()
	h.hostname = hostname
	h.port = port

	allDstData := h.getDataFromHost(allSrcData, sslFlag)

	if allDstData != nil {
		h.sslClientServerClient(allDstData)
	} else {
		err := h.connSrc.Close()
		if err != nil {
			return
		}
	}
}

func (h *MyHost) getDataFromHost(allSrcData []byte, sslFlag bool) []byte {
	var err error
	h.connDst, err = net.Dial("tcp", fmt.Sprintf("%s:%d", h.hostname, h.port))
	if err != nil {
		if h.connDst != nil {
			err := h.connDst.Close()
			if err != nil {
				return nil
			}
		}
		return nil
	}

	if sslFlag {
		return []byte("HTTP/1.0 200 Connection Established\r\n\r\n")
	}

	_, err = h.connDst.Write(allSrcData)
	if err != nil {
		err := h.connDst.Close()
		if err != nil {
			return nil
		}
		return nil
	}

	rcData := make([]byte, cfg.CommonConf.BufferSize)
	n, err := h.connDst.Read(rcData)
	if err != nil {
		err := h.connDst.Close()
		if err != nil {
			return nil
		}
		return nil
	}

	return rcData[:n]
}

func (h *MyHost) sslClientServerClient(allDstData []byte) {
	allDstData = data.DownCompress(allDstData, cfg.CommonConf.Crypt) // Assuming DownCompress is a method in util
	if !h.checkMyChain() {
		return
	}

	_, err := h.connSrc.Write(allDstData)
	if err != nil {
		h.closeMyChain()
		return
	}

	h.wg.Add(2)
	go h.sslClientServer()
	go h.sslServerClient()
}

func (h *MyHost) sslClientServer() {
	defer h.wg.Done()
	for {
		if !h.checkMyChain() {
			return
		}

		sslClientData := make([]byte, cfg.CommonConf.BufferSize)
		n, err := h.connSrc.Read(sslClientData)
		if err != nil {
			h.closeMyChain()
			return
		}

		if n > 0 {
			sslClientData = data.UpDecompress(sslClientData[:n], cfg.CommonConf.Crypt) // Assuming UpDecompress is a method in util
			if !h.checkMyChain() {
				return
			}
			_, err := h.connDst.Write(sslClientData)
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

func (h *MyHost) sslServerClient() {
	defer h.wg.Done()
	for {
		if !h.checkMyChain() {
			return
		}

		sslServerData := make([]byte, cfg.CommonConf.BufferSize)
		n, err := h.connDst.Read(sslServerData)
		if err != nil {
			h.closeMyChain()
			return
		}

		if n > 0 {
			sslServerData = data.DownCompress(sslServerData[:n], cfg.CommonConf.Crypt) // Assuming DownCompress is a method in util
			if !h.checkMyChain() {
				return
			}
			_, err := h.connSrc.Write(sslServerData)
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
	if h.connSrc != nil && h.connDst != nil {
		return true
	}
	h.closeMyChain()
	return false
}

func (h *MyHost) closeMyChain() {
	err := h.connSrc.Close()
	if err != nil {
		return
	}
	err = h.connDst.Close()
	if err != nil {
		return
	}

}

func (h *MyHost) getDstHostFromHeader() ([]byte, string, int, bool) {
	var header []byte
	//sslFlag := false

	for {
		// 读取头部数据
		line := make([]byte, cfg.CommonConf.BufferSize)
		n, err := h.connSrc.Read(line)

		if err != nil {
			fmt.Printf("Error reading header: %v\n", err)
			return nil, "", 0, false
		}

		if n <= 0 {
			break
		}
		line = line[:n]
		header = append(header, line...)
		// fmt.Printf("crypt:%d header:%+v \n", cfg.CommonConf.Crypt, header)
		header = data.UpDecompressHeader(header, cfg.CommonConf.Crypt)

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
			//fmt.Println(string(header))
			//GET http://5g.people.cn/background.png HTTP/1.1
			//Host: 5g.people.cn
			//Proxy-Connection: keep-alive
			//...
			if hostIndex > -1 {
				hostLine := header[hostIndex:]
				endOfLine := bytes.Index(hostLine, []byte("\n"))
				if endOfLine < 5 {
					return nil, "", 0, false
				}
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
