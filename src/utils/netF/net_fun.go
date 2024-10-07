package netF

import (
	"fmt"
	"net"
)

func CloseConnWithInfo(conn net.Conn, showinfo ...any) {
	if showinfo != nil {
		fmt.Println(showinfo...)
	}
	err := conn.Close()
	if err != nil {
		return
	}
	return
}

func CloseConnection(conn net.Conn) {
	err := conn.Close()
	if err != nil {
		return
	}
	return
}
