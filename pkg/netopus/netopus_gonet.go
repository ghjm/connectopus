package netopus

import (
	"context"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv6"
	"net"
)

// DialTCP dials a TCP connection over the Netopus network
func (n *netopus) DialTCP(addr net.IP, port uint16) (net.Conn, error) {
	return gonet.DialTCP(
		n.stack.Stack,
		tcpip.FullAddress{
			NIC:  1,
			Addr: tcpip.Address(addr),
			Port: port,
		},
		ipv6.ProtocolNumber)
}

// DialContextTCP dials a TCP connection over the Netopus network, using a context
func (n *netopus) DialContextTCP(ctx context.Context, addr net.IP, port uint16) (net.Conn, error) {
	return gonet.DialContextTCP(ctx,
		n.stack.Stack,
		tcpip.FullAddress{
			NIC:  1,
			Addr: tcpip.Address(addr),
			Port: port,
		},
		ipv6.ProtocolNumber)
}

// ListenTCP opens a TCP listener over the Netopus network
func (n *netopus) ListenTCP(port uint16) (net.Listener, error) {
	return gonet.ListenTCP(
		n.stack.Stack,
		tcpip.FullAddress{
			NIC:  1,
			Addr: tcpip.Address(n.addr),
			Port: port,
		},
		ipv6.ProtocolNumber)
}

// DialUDP opens a UDP sender or receiver over the Netopus network
func (n *netopus) DialUDP(lport uint16, addr net.IP, rport uint16) (UDPConn, error) {
	var laddr *tcpip.FullAddress
	if lport != 0 {
		laddr = &tcpip.FullAddress{
			NIC:  1,
			Addr: tcpip.Address(n.addr),
			Port: lport,
		}
	}
	var raddr *tcpip.FullAddress
	if addr != nil {
		raddr = &tcpip.FullAddress{
			NIC:  1,
			Addr: tcpip.Address(addr),
			Port: rport,
		}
	}
	return gonet.DialUDP(n.stack.Stack, laddr, raddr, ipv6.ProtocolNumber)
}
