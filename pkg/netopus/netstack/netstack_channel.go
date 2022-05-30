package netstack

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/pkg/x/broker"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/buffer"
	"gvisor.dev/gvisor/pkg/tcpip/link/channel"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv6"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/icmp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/udp"
	"io"
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

// NewStackChannel creates a new network stack
func NewStackChannel(ctx context.Context, subnet *net.IPNet, addr net.IP) (NetStack, error) {
	if len(addr) != net.IPv6len || len(subnet.IP) != net.IPv6len {
		return nil, fmt.Errorf("subnet and address must be ipv6")
	}
	if !subnet.Contains(addr) {
		return nil, fmt.Errorf("%s is not in the subnet %s", addr.String(), subnet.String())
	}
	ns := &netStackChannel{
		addr:         addr,
		packetBroker: broker.NewBroker[[]byte](ctx),
	}
	ns.stack = stack.New(stack.Options{
		NetworkProtocols: []stack.NetworkProtocolFactory{ipv4.NewProtocol, ipv6.NewProtocol},
		TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol, udp.NewProtocol,
			icmp.NewProtocol4, icmp.NewProtocol6},
		HandleLocal: true,
	})
	ns.endpoint = channel.New(16, 1500, "11:11:11:11:11:11")
	ns.stack.CreateNICWithOptions(1, ns.endpoint, stack.NICOptions{
		Name:     "net0",
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
	maskOnes, _ := subnet.Mask.Size()
	localNet := tcpip.AddressWithPrefix{
		Address:   tcpip.Address(subnet.IP),
		PrefixLen: maskOnes,
	}
	ns.stack.AddRoute(tcpip.Route{
		Destination: localNet.Subnet(),
		NIC:         1,
	})

	// Clean up after termination
	go func() {
		<-ctx.Done()
		ns.endpointLock.Lock()
		defer ns.endpointLock.Unlock()
		ns.endpoint.Attach(nil)
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
		totalSize := 0
		for _, v := range op.Views() {
			totalSize += v.Size()
		}
		packet := make([]byte, totalSize)
		var err error
		curPos := 0
		for _, v := range op.Views() {
			var n int
			r := v.Reader()
			n, err = r.Read(packet[curPos:])
			if err == nil && n != v.Size() {
				err = fmt.Errorf("expected %d bytes but read %d", v.Size(), n)
			}
			if err != nil && err != io.EOF {
				break
			}
			curPos += v.Size()
		}
		op.DecRef()
		if err != nil && err != io.EOF {
			continue
		}
		ns.packetBroker.Publish(packet)
	}
}

func (ns *netStackChannel) SendPacket(packet []byte) error {
	buf := buffer.NewView(len(packet))
	copy(buf[:], packet)
	pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{
		Data: buf.ToVectorisedView(),
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
			Addr: tcpip.Address(addr),
			Port: port,
		},
		ipv6.ProtocolNumber)
}

func (ns *netStackChannel) DialContextTCP(ctx context.Context, addr net.IP, port uint16) (net.Conn, error) {
	return gonet.DialContextTCP(ctx,
		ns.stack,
		tcpip.FullAddress{
			NIC:  1,
			Addr: tcpip.Address(addr),
			Port: port,
		},
		ipv6.ProtocolNumber)
}

func (ns *netStackChannel) ListenTCP(port uint16) (net.Listener, error) {
	return gonet.ListenTCP(
		ns.stack,
		tcpip.FullAddress{
			NIC:  1,
			Addr: tcpip.Address(ns.addr),
			Port: port,
		},
		ipv6.ProtocolNumber)
}

func (ns *netStackChannel) DialUDP(lport uint16, raddr net.IP, rport uint16) (UDPConn, error) {
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
