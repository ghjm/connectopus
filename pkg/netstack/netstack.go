package netstack

import (
	"context"
	"github.com/ghjm/connectopus/pkg/links"
	"net"
)

// UDPConn is functionally equivalent to net.UDPConn
type UDPConn interface {
	net.Conn
	net.PacketConn
}

// UserStack provides the methods that allow user apps to communicate over a network stack
type UserStack interface {
	// DialTCP dials a TCP connection over the network stack.
	DialTCP(addr net.IP, port uint16) (net.Conn, error)
	// DialContextTCP dials a TCP connection over the network stack, using a context.
	DialContextTCP(ctx context.Context, addr net.IP, port uint16) (net.Conn, error)
	// ListenTCP opens a TCP listener over the network stack.
	ListenTCP(port uint16) (net.Listener, error)
	// DialUDP opens a UDP sender or receiver over the network stack.  If addr is nil,
	// rport will be ignored and this socket will only listen on lport.
	DialUDP(lport uint16, addr net.IP, rport uint16) (UDPConn, error)
}

// NetStack represents an IPv6 network stack
type NetStack interface {
	links.Link
	UserStack
}

// NewStackFunc is the type of a function that creates a new stack
type NewStackFunc func(context.Context, net.IP) (NetStack, error)
