package netstack

import (
	"context"
	"github.com/ghjm/connectopus/pkg/utils/broker"
	log "github.com/sirupsen/logrus"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/link/fdbased"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv6"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/udp"
	"net"
	"syscall"
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
	// DialUDP opens a UDP sender or receiver over the network stack.  If raddr is nil,
	// rport will be ignored and this socket will only listen on lport.
	DialUDP(lport uint16, addr net.IP, rport uint16) (UDPConn, error)
}

// NetStack represents an IPv6 network stack
type NetStack interface {
	PacketStack
	UserStack
}

// netStack implements NetStack, using gVisor
type netStack struct {
	addr         net.IP
	stack        *stack.Stack
	endpoint     stack.LinkEndpoint
	packetBroker broker.Broker[[]byte]
	fds          [2]int
}

// NewStack creates a new network stack
func NewStack(ctx context.Context, addr net.IP) (NetStack, error) {
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_SEQPACKET, 0)
	if err != nil {
		return nil, err
	}
	ns := &netStack{
		addr:         addr,
		packetBroker: broker.NewBroker[[]byte](ctx),
		fds:          fds,
	}
	ns.stack = stack.New(stack.Options{
		NetworkProtocols:   []stack.NetworkProtocolFactory{ipv4.NewProtocol, ipv6.NewProtocol},
		TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol, udp.NewProtocol},
		HandleLocal:        true,
	})
	ns.endpoint, err = fdbased.New(&fdbased.Options{
		FDs: []int{fds[0]},
		MTU: 1500,
		ClosedFunc: func(err tcpip.Error) {
			if err != nil {
				log.Errorf("netstack closed with error: %s", err)
			}
		},
	})
	if err != nil {
		return nil, err
	}
	ns.stack.CreateNICWithOptions(1, ns.endpoint, stack.NICOptions{
		Name:     "1",
		Disabled: false,
	})
	ns.stack.AddProtocolAddress(1,
		tcpip.ProtocolAddress{
			Protocol: ipv6.ProtocolNumber,
			AddressWithPrefix: tcpip.AddressWithPrefix{
				Address:   tcpip.Address(addr),
				PrefixLen: 128,
			},
		},
		stack.AddressProperties{},
	)
	localNet := tcpip.AddressWithPrefix{
		Address:   tcpip.Address(net.ParseIP("FD00::0")),
		PrefixLen: 8,
	}
	ns.stack.AddRoute(tcpip.Route{
		Destination: localNet.Subnet(),
		NIC:         1,
	})

	go func() {
		<-ctx.Done()
		ns.endpoint.Attach(nil)
		ns.stack.Close()
		ns.endpoint.Wait()
		_ = syscall.Close(ns.fds[0])
		_ = syscall.Close(ns.fds[1])
	}()

	// Send incoming packets to subscribed receivers
	go func() {
		for {
			packet := make([]byte, ns.endpoint.MTU())
			n, err := syscall.Read(ns.fds[1], packet)
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				log.Errorf("error reading from stack endpoint fd: %s", err)
			}
			packet = packet[:n]
			go ns.packetBroker.Publish(packet)
		}
	}()

	return ns, nil
}

func (ns *netStack) SendPacket(packet []byte) error {
	_, err := syscall.Write(ns.fds[1], packet)
	return err
}

func (ns *netStack) SubscribePackets() <-chan []byte {
	return ns.packetBroker.Subscribe()
}

func (ns *netStack) UnsubscribePackets(pktCh <-chan []byte) {
	ns.packetBroker.Unsubscribe(pktCh)
}

func (ns *netStack) DialTCP(addr net.IP, port uint16) (net.Conn, error) {
	return gonet.DialTCP(
		ns.stack,
		tcpip.FullAddress{
			NIC:  1,
			Addr: tcpip.Address(addr),
			Port: port,
		},
		ipv6.ProtocolNumber)
}

func (ns *netStack) DialContextTCP(ctx context.Context, addr net.IP, port uint16) (net.Conn, error) {
	return gonet.DialContextTCP(ctx,
		ns.stack,
		tcpip.FullAddress{
			NIC:  1,
			Addr: tcpip.Address(addr),
			Port: port,
		},
		ipv6.ProtocolNumber)
}

func (ns *netStack) ListenTCP(port uint16) (net.Listener, error) {
	return gonet.ListenTCP(
		ns.stack,
		tcpip.FullAddress{
			NIC:  1,
			Addr: tcpip.Address(ns.addr),
			Port: port,
		},
		ipv6.ProtocolNumber)
}

func (ns *netStack) DialUDP(lport uint16, raddr net.IP, rport uint16) (UDPConn, error) {
	var lfaddr *tcpip.FullAddress
	if lport != 0 {
		lfaddr = &tcpip.FullAddress{
			NIC:  1,
			Addr: tcpip.Address(ns.addr),
			Port: lport,
		}
	}
	var rfaddr *tcpip.FullAddress
	if raddr != nil {
		rfaddr = &tcpip.FullAddress{
			NIC:  1,
			Addr: tcpip.Address(raddr),
			Port: rport,
		}
	}
	return gonet.DialUDP(ns.stack, lfaddr, rfaddr, ipv6.ProtocolNumber)
}
