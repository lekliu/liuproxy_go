// --- START OF COMPLETE REPLACEMENT for liuproxy_go/internal/goremote/helpers.go ---
package goremote

import (
	"bytes"
	"encoding/binary"
	"net"
	"strconv"
)

// buildMetadata (包内私有)
func buildMetadata(cmd byte, targetAddr string) []byte {
	host, portStr, _ := net.SplitHostPort(targetAddr)
	port, _ := strconv.Atoi(portStr)
	addrBytes := []byte(host)
	addrType := byte(0x03)
	if ip := net.ParseIP(host); ip != nil {
		if ipv4 := ip.To4(); ipv4 != nil {
			addrType = 0x01
			addrBytes = ipv4
		} else {
			addrType = 0x04
			addrBytes = ip.To16()
		}
	}
	var buf bytes.Buffer
	buf.WriteByte(cmd)
	buf.WriteByte(addrType)
	if addrType == 0x03 {
		buf.WriteByte(byte(len(addrBytes)))
	}
	buf.Write(addrBytes)
	_ = binary.Write(&buf, binary.BigEndian, uint16(port))
	return buf.Bytes()
}
