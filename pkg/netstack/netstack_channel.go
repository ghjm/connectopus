package netstack

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/pkg/x/broker"
	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/link/channel"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv6"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/icmp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/udp"
	"net"
	"sync"
)

// netStackChannel implements NetStack, using gVisor with a channel endpoint
type netStackChannel struct {
	addr         net.IP
	stack        *stack.Stack
	endpoint     *channel.Endpoint
	endpointLock sync.RWMutex
	packetBroker broker.Broker[[]byte]
}

func (ns *netStackChannel) MTU() uint16 {
	return uint16(ns.endpoint.MTU())
}

// NewStackChannel creates a new network stack
func NewStackChannel(ctx context.Context, addr net.IP, mtu uint16) (NetStack, error) {
	if len(addr) != net.IPv6len {
		return nil, fmt.Errorf("address must be ipv6")
	}
	ns := &netStackChannel{
		addr:         addr,
		packetBroker: broker.New[[]byte](ctx),
	}
	ns.stack = stack.New(stack.Options{
		NetworkProtocols: []stack.NetworkProtocolFactory{ipv4.NewProtocol, ipv6.NewProtocol},
		TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol, udp.NewProtocol,
			icmp.NewProtocol4, icmp.NewProtocol6},
		HandleLocal: true,
	})
	ns.endpoint = channel.New(16, uint32(mtu), "11:11:11:11:11:11")
	ns.stack.CreateNICWithOptions(1, ns.endpoint, stack.NICOptions{
		Name:     "net0",
		Disabled: false,
	})
	ns.stack.AddProtocolAddress(1,
		tcpip.ProtocolAddress{
			Protocol: ipv6.ProtocolNumber,
			AddressWithPrefix: tcpip.AddressWithPrefix{
				Address:   tcpip.AddrFromSlice(addr),
				PrefixLen: 128,
			},
		},
		stack.AddressProperties{},
	)
	localNet := tcpip.AddressWithPrefix{
		Address:   tcpip.AddrFromSlice(addr),
		PrefixLen: len(addr) * 8,
	}
	ns.stack.AddRoute(tcpip.Route{
		Destination: localNet.Subnet(),
		NIC:         1,
	})
	defaultNet := tcpip.AddressWithPrefix{
		Address:   tcpip.AddrFromSlice(addr),
		PrefixLen: 0,
	}
	ns.stack.AddRoute(tcpip.Route{
		Destination: defaultNet.Subnet(),
		NIC:         1,
	})

	// Clean up after termination
	go func() {
		<-ctx.Done()
		ns.endpointLock.Lock()
		defer ns.endpointLock.Unlock()
		ns.stack.Close()
		ns.endpoint.Wait()
	}()

	// Send incoming packets to subscribed receivers
	go ns.packetPublisher(ctx)

	return ns, nil
}

// packetPublisher publishes outgoing packets from the stack to the packetBroker
func (ns *netStackChannel) packetPublisher(ctx context.Context) {
	for {
		op := ns.endpoint.ReadContext(ctx)
		if ctx.Err() != nil {
			// Shut down gVisor goroutines on exit
			ns.endpoint.Close()
			ns.stack.Close()
			return
		}
		if op == nil {
			continue
		}
		buf := op.ToBuffer()
		packet := (&buf).Flatten()
		op.DecRef()
		ns.packetBroker.Publish(packet)
	}
}

func (ns *netStackChannel) SendPacket(packet []byte) error {
	pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{
		Payload: buffer.MakeWithView(buffer.NewViewWithData(packet)),
	})
	ns.endpointLock.RLock()
	defer ns.endpointLock.RUnlock()
	ns.endpoint.InjectInbound(ipv6.ProtocolNumber, pkt)
	return nil
}

func (ns *netStackChannel) SubscribePackets() <-chan []byte {
	return ns.packetBroker.Subscribe()
}

func (ns *netStackChannel) UnsubscribePackets(pktCh <-chan []byte) {
	ns.packetBroker.Unsubscribe(pktCh)
}

func (ns *netStackChannel) DialTCP(addr net.IP, port uint16) (net.Conn, error) {
	return gonet.DialTCP(
		ns.stack,
		tcpip.FullAddress{
			NIC:  1,
			Addr: tcpip.AddrFromSlice(addr),
			Port: port,
		},
		ipv6.ProtocolNumber)
}

func (ns *netStackChannel) DialContextTCP(ctx context.Context, addr net.IP, port uint16) (net.Conn, error) {
	return gonet.DialContextTCP(ctx,
		ns.stack,
		tcpip.FullAddress{
			NIC:  1,
			Addr: tcpip.AddrFromSlice(addr),
			Port: port,
		},
		ipv6.ProtocolNumber)
}

func (ns *netStackChannel) ListenTCP(port uint16) (net.Listener, error) {
	return gonet.ListenTCP(
		ns.stack,
		tcpip.FullAddress{
			NIC:  1,
			Addr: tcpip.AddrFromSlice(ns.addr),
			Port: port,
		},
		ipv6.ProtocolNumber)
}

func (ns *netStackChannel) DialUDP(lport uint16, raddr net.IP, rport uint16) (UDPConn, error) {
	var lfaddr *tcpip.FullAddress
	if lport != 0 {
		lfaddr = &tcpip.FullAddress{
			NIC:  1,
			Addr: tcpip.AddrFromSlice(ns.addr),
			Port: lport,
		}
	}
	var rfaddr *tcpip.FullAddress
	if raddr != nil {
		rfaddr = &tcpip.FullAddress{
			NIC:  1,
			Addr: tcpip.AddrFromSlice(raddr),
			Port: rport,
		}
	}
	return gonet.DialUDP(ns.stack, lfaddr, rfaddr, ipv6.ProtocolNumber)
}
