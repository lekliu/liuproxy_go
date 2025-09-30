package session

import (
	"liuproxy_go/internal/xray_core/common/net"
)

// ID of a session.
type ID uint32

// Inbound is the metadata of an inbound connection.
type Inbound struct {
	// Source address of the inbound connection.
	Source net.Destination
	Conn   net.Conn
	// CanSpliceCopy is a property for this connection, set by both inbound and outbound
	// 1 = can, 2 = after processing protocol info should be able to, 3 = cannot
	CanSpliceCopy int
}

// Outbound is the metadata of an outbound connection.
type Outbound struct {
	// Target address of the outbound connection.
	OriginalTarget net.Destination
	Target         net.Destination
	RouteTarget    net.Destination
	// Gateway address
	Gateway net.Address
	// Conn is actually internet.Connection. May be nil. It is currently nil for outbound with proxySettings
	Conn net.Conn
}
