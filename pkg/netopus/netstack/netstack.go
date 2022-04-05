package netstack

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/pkg/utils/pubsub"
	"gvisor.dev/gvisor/pkg/tcpip"
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
)

// NetStack represents a gVisor network stack
type NetStack struct {
	Addr       net.IP
	Stack      *stack.Stack
	Endpoint   *channel.Endpoint
	recvBroker pubsub.Broker[[]byte]
}

// NewStack creates a new gVisor network stack
func NewStack(ctx context.Context, addr net.IP) *NetStack {
	ns := &NetStack{
		Addr:       addr,
		recvBroker: pubsub.NewBroker[[]byte](ctx),
	}
	ns.Stack = stack.New(stack.Options{
		NetworkProtocols:   []stack.NetworkProtocolFactory{ipv4.NewProtocol, ipv6.NewProtocol},
		TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol, udp.NewProtocol, icmp.NewProtocol6},
		HandleLocal:        true,
	})
	ns.Endpoint = channel.New(16, 1500, "AAAAAA")
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
		Gateway:     tcpip.Address(addr),
		NIC:         1,
	})

	// Send incoming packets to subscribed receivers
	go func() {
		for {
			op := ns.Endpoint.ReadContext(ctx)
			if ctx.Err() != nil {
				// Shut down gVisor goroutines on exit
				ns.Endpoint.Close()
				ns.Stack.Close()
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
			if err != nil {
				continue
			}
			ns.recvBroker.Publish(packet, pubsub.NoWait)
		}
	}()

	return ns
}

// InjectPacket injects a single packet into the network stack.  The data must be a valid IPv6 packet.
func (ns *NetStack) InjectPacket(packet []byte) {
	pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{
		Data: buffer.View(packet).ToVectorisedView(),
	})
	ns.Endpoint.InjectInbound(ipv6.ProtocolNumber, pkt)
	pkt.DecRef()
}

// SubscribePackets establishes a channel that will receive outgoing packets from the stack.
func (ns *NetStack) SubscribePackets() <-chan []byte {
	return ns.recvBroker.Subscribe()
}

// UnsubscribePackets releases a previously subscribed channel.
func (ns *NetStack) UnsubscribePackets(c <-chan []byte) {
	ns.recvBroker.Unsubscribe(c)
}
