package netstack

import (
	"context"
	"github.com/ghjm/connectopus/pkg/utils/broker"
	log "github.com/sirupsen/logrus"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/link/fdbased"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv6"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/udp"
	"net"
	"syscall"
)

// NetStack represents a gVisor network stack
type NetStack struct {
	Addr         net.IP
	Stack        *stack.Stack
	Endpoint     stack.LinkEndpoint
	packetBroker broker.Broker[[]byte]
	fds          [2]int
}

// NewStack creates a new gVisor network stack
func NewStack(ctx context.Context, addr net.IP) (*NetStack, error) {
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_SEQPACKET, 0)
	if err != nil {
		return nil, err
	}
	ns := &NetStack{
		Addr:         addr,
		packetBroker: broker.NewBroker[[]byte](ctx),
		fds:          fds,
	}
	ns.Stack = stack.New(stack.Options{
		NetworkProtocols:   []stack.NetworkProtocolFactory{ipv4.NewProtocol, ipv6.NewProtocol},
		TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol, udp.NewProtocol},
		HandleLocal:        true,
	})
	ns.Endpoint, err = fdbased.New(&fdbased.Options{
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
	ns.Stack.CreateNICWithOptions(1, ns.Endpoint, stack.NICOptions{
		Name:     "1",
		Disabled: false,
	})
	ns.Stack.AddProtocolAddress(1,
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
	ns.Stack.AddRoute(tcpip.Route{
		Destination: localNet.Subnet(),
		NIC:         1,
	})

	go func() {
		<-ctx.Done()
		ns.Endpoint.Attach(nil)
		ns.Stack.Close()
		ns.Endpoint.Wait()
		_ = syscall.Close(ns.fds[0])
		_ = syscall.Close(ns.fds[1])
	}()

	// Send incoming packets to subscribed receivers
	go func() {
		for {
			packet := make([]byte, ns.Endpoint.MTU())
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

// SendPacket injects a single packet into the network stack.  The data must be a valid IPv6 packet.
func (ns *NetStack) SendPacket(packet []byte) error {
	_, err := syscall.Write(ns.fds[1], packet)
	return err
}

// SubscribePackets returns a channel which will receive packets outgoing from the network stack.
func (ns *NetStack) SubscribePackets() <-chan []byte {
	return ns.packetBroker.Subscribe()
}

// UnsubscribePackets unsubscribes a channel previously subscribed with SubscribePackets
func (ns *NetStack) UnsubscribePackets(pktCh <-chan []byte) {
	ns.packetBroker.Unsubscribe(pktCh)
}
