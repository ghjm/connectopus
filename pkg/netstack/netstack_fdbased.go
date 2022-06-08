//go:build linux

package netstack

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/pkg/x/broker"
	log "github.com/sirupsen/logrus"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/link/fdbased"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv6"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/icmp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/udp"
	"net"
	"syscall"
)

// netStackFdbased implements NetStack, using gVisor with an fdbased endpoint
type netStackFdbased struct {
	addr         net.IP
	stack        *stack.Stack
	endpoint     stack.LinkEndpoint
	packetBroker broker.Broker[[]byte]
	fds          [2]int
}

// NewStackFdbased creates a new fdbased network stack
func NewStackFdbased(ctx context.Context, addr net.IPNet) (NetStack, error) {
	if len(addr.IP) != net.IPv6len {
		return nil, fmt.Errorf("address must be ipv6")
	}
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_SEQPACKET, 0)
	if err != nil {
		return nil, err
	}
	ns := &netStackFdbased{
		addr:         addr.IP,
		packetBroker: broker.New[[]byte](ctx),
		fds:          fds,
	}
	ns.stack = stack.New(stack.Options{
		NetworkProtocols: []stack.NetworkProtocolFactory{ipv4.NewProtocol, ipv6.NewProtocol},
		TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol, udp.NewProtocol,
			icmp.NewProtocol4, icmp.NewProtocol6},
		HandleLocal: true,
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
		Name:     "net0",
		Disabled: false,
	})
	ns.stack.AddProtocolAddress(1,
		tcpip.ProtocolAddress{
			Protocol: ipv6.ProtocolNumber,
			AddressWithPrefix: tcpip.AddressWithPrefix{
				Address:   tcpip.Address(addr.IP),
				PrefixLen: 128,
			},
		},
		stack.AddressProperties{},
	)
	maskOnes, _ := addr.Mask.Size()
	localNet := tcpip.AddressWithPrefix{
		Address:   tcpip.Address(addr.IP),
		PrefixLen: maskOnes,
	}
	ns.stack.AddRoute(tcpip.Route{
		Destination: localNet.Subnet(),
		NIC:         1,
	})

	// Clean up after termination
	go func() {
		<-ctx.Done()
		ns.endpoint.Attach(nil)
		ns.stack.Close()
		ns.endpoint.Wait()
		_ = syscall.Close(ns.fds[0])
		_ = syscall.Close(ns.fds[1])
	}()

	// Send incoming packets to subscribed receivers
	go ns.packetPublisher(ctx)

	return ns, nil
}

// packetPublisher publishes outgoing packets from the stack to the packetBroker
func (ns *netStackFdbased) packetPublisher(ctx context.Context) {
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
}

func (ns *netStackFdbased) SendPacket(packet []byte) error {
	_, err := syscall.Write(ns.fds[1], packet)
	return err
}

func (ns *netStackFdbased) SubscribePackets() <-chan []byte {
	return ns.packetBroker.Subscribe()
}

func (ns *netStackFdbased) UnsubscribePackets(pktCh <-chan []byte) {
	ns.packetBroker.Unsubscribe(pktCh)
}

func (ns *netStackFdbased) DialTCP(addr net.IP, port uint16) (net.Conn, error) {
	return gonet.DialTCP(
		ns.stack,
		tcpip.FullAddress{
			NIC:  1,
			Addr: tcpip.Address(addr),
			Port: port,
		},
		ipv6.ProtocolNumber)
}

func (ns *netStackFdbased) DialContextTCP(ctx context.Context, addr net.IP, port uint16) (net.Conn, error) {
	return gonet.DialContextTCP(ctx,
		ns.stack,
		tcpip.FullAddress{
			NIC:  1,
			Addr: tcpip.Address(addr),
			Port: port,
		},
		ipv6.ProtocolNumber)
}

func (ns *netStackFdbased) ListenTCP(port uint16) (net.Listener, error) {
	return gonet.ListenTCP(
		ns.stack,
		tcpip.FullAddress{
			NIC:  1,
			Addr: tcpip.Address(ns.addr),
			Port: port,
		},
		ipv6.ProtocolNumber)
}

func (ns *netStackFdbased) DialUDP(lport uint16, raddr net.IP, rport uint16) (UDPConn, error) {
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
