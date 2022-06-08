package netstack

import (
	"context"
	"fmt"
	"net"
)

// NetUserStack provides a UserStack that is a thin wrapper around the Go net library.  Used for testing.
type NetUserStack struct{}

func (nus *NetUserStack) DialTCP(addr net.IP, port uint16) (net.Conn, error) {
	return net.Dial("tcp", fmt.Sprintf("%s:%d", addr.String(), port))
}

func (nus *NetUserStack) DialContextTCP(ctx context.Context, addr net.IP, port uint16) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", addr.String(), port))
}

func (nus *NetUserStack) ListenTCP(port uint16) (net.Listener, error) {
	return net.Listen("tcp", fmt.Sprintf(":%d", port))
}

func (nus *NetUserStack) DialUDP(lport uint16, addr net.IP, rport uint16) (UDPConn, error) {
	var laddr *net.UDPAddr
	if lport != 0 {
		laddr = &net.UDPAddr{
			IP:   nil,
			Port: int(lport),
		}
	}
	raddr := &net.UDPAddr{
		IP:   addr,
		Port: int(rport),
	}
	return net.DialUDP("udp", laddr, raddr)
}
