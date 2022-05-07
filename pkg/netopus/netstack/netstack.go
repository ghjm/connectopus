package netstack

import (
	"context"
	"net"
)

// UDPConn is functionally equivalent to net.UDPConn
type UDPConn interface {
	net.Conn
	net.PacketConn
}

// PacketStack is a network stack that accepts and produces IPv6 packets
type PacketStack interface {
	// SendPacket injects a single packet into the network stack.  The data must be a valid IPv6 packet.
	SendPacket(packet []byte) error
	// SubscribePackets returns a channel which will receive packets outgoing from the network stack.
	SubscribePackets() <-chan []byte
	// UnsubscribePackets unsubscribes a channel previously subscribed with SubscribePackets.
	UnsubscribePackets(pktCh <-chan []byte)
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
	PacketStack
	UserStack
}

// NewStackFunc is the type of a function that creates a new stack
type NewStackFunc func(context.Context, *net.IPNet, net.IP) (NetStack, error)
