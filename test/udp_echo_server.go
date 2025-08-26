// file: test/udp_echo_server.go
package main

import (
	"log"
	"net"
)

func main() {
	// 在所有网络接口的 4000 端口上监听UDP包
	addr, err := net.ResolveUDPAddr("udp", ":4000")
	if err != nil {
		log.Fatalf("无法解析地址: %v", err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Fatalf("无法监听UDP端口: %v", err)
	}
	defer conn.Close()

	log.Println("UDP Echo Server 正在监听 :4000 ...")

	buf := make([]byte, 2048)
	for {
		// 读取一个UDP包
		n, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			log.Printf("读取错误: %v", err)
			continue
		}

		// 打印收到的内容
		log.Printf("收到来自 %s 的 %d 字节: %s", remoteAddr, n, string(buf[:n]))

		// 将收到的数据原封不动地发回给源地址
		_, err = conn.WriteToUDP(buf[:n], remoteAddr)
		if err != nil {
			log.Printf("写入错误: %v", err)
		} else {
			log.Printf("已将 %d 字节数据回显给 %s", n, remoteAddr)
		}
	}
}
