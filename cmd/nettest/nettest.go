package main

import (
	"bytes"
	"context"
	"fmt"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/buffer"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/link/channel"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv6"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/udp"
	"io/ioutil"
	"net"
	"strings"
	"time"
)

func NetTest() error {
	s := stack.New(stack.Options{
		NetworkProtocols:         []stack.NetworkProtocolFactory{ipv4.NewProtocol, ipv6.NewProtocol},
		TransportProtocols:       []stack.TransportProtocolFactory{tcp.NewProtocol, udp.NewProtocol},
		HandleLocal:              true,
	})

	ma, err := tcpip.ParseMACAddress("11:22:33:44:55:66")
	if err != nil {
		return err
	}
	ep := channel.New(16, 1500, ma)
	s.CreateNICWithOptions(1, ep, stack.NICOptions{
		Name:     "1",
		Disabled: false,
	})

	s.AddProtocolAddress(1,
		tcpip.ProtocolAddress{
			Protocol:          ipv6.ProtocolNumber,
			AddressWithPrefix: tcpip.AddressWithPrefix{
				Address:   header.IPv6Loopback,
				PrefixLen: 128,
			},
		},
		stack.AddressProperties{},
	)

	mySubnetStr := "fd12:3456:789a:1::"
	mySubnet := tcpip.Address(net.ParseIP(mySubnetStr))
	myAddr := tcpip.Address(net.ParseIP(mySubnetStr+"1"))
	remoteAddr := tcpip.Address(net.ParseIP(mySubnetStr+"2"))

	s.AddProtocolAddress(1,
		tcpip.ProtocolAddress{
			Protocol:          ipv6.ProtocolNumber,
			AddressWithPrefix: tcpip.AddressWithPrefix{
				Address:   myAddr,
				PrefixLen: 64,
			},
		},
		stack.AddressProperties{},
	)

	tcpFA := tcpip.FullAddress{
		NIC:  1,
		Addr: header.IPv6Loopback,
		Port: 1234,
	}
	li, err := gonet.ListenTCP(s, tcpFA, ipv6.ProtocolNumber)
	if err != nil {
		return err
	}
	go func() {
		c, err := li.Accept()
		if err != nil {
			return
		}
		_, _ = c.Write([]byte("Hello, world!\n"))
		_ = c.Close()
	}()

	c, err := gonet.DialTCP(s, tcpFA, ipv6.ProtocolNumber)
	if err != nil {
		return err
	}
	b, err := ioutil.ReadAll(c)
	if err != nil {
		return err
	}
	fmt.Printf("TCP test received: %s", string(b))
	_ = c.Close()

	udpFA := tcpip.FullAddress{
		NIC:  1,
		Addr: myAddr,
		Port: 1234,
	}
	udpc, err := gonet.DialUDP(s, &udpFA, nil, ipv6.ProtocolNumber)
	if err != nil {
		return err
	}
	go func() {
		buf := make([]byte, 1500)
		for {
			n, addr, rerr := udpc.ReadFrom(buf)
			if rerr != nil {
				fmt.Printf("UDP read error: %s\n", rerr)
				return
			}
			fmt.Printf("UDP %d bytes from %v: %s\n", n, addr, buf[:n])
		}
	}()

	subnet, err := tcpip.NewSubnet(mySubnet,
		tcpip.AddressMask(strings.Repeat("\xFF", 8) + strings.Repeat("\x00", 8)))
	if err != nil {
		return err
	}
	s.AddRoute(tcpip.Route{
		Destination: subnet,
		Gateway:     myAddr,
		NIC:         1,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		for {
			op := ep.ReadContext(ctx)
			if op == nil {
				continue
			}
			pkt := op.DeepCopyForForwarding(0)
			op.DecRef()

			pktNet := pkt.Network()

			thv := pkt.TransportHeader().View()
			var thbuf []byte
			if thv != nil {
				thbuf = make([]byte, thv.Size())
				thvv := thv.ToVectorisedView()
				_, err = thvv.Read(thbuf)
				if err != nil {
					fmt.Printf("Transport header read error: %s\n", err)
					return
				}
			}
			payload := new(bytes.Buffer)
			_, err = pkt.Data().ReadTo(payload, false)
			if err != nil {
				fmt.Printf("Data read error: %s\n", err)
				return
			}
			fmt.Printf("Packet:\n")
			fmt.Printf("  Type:        %v\n", pkt.PktType)
			fmt.Printf("  Transport:   %d\n", pktNet.TransportProtocol())
			fmt.Printf("  From:        %v\n", pktNet.SourceAddress())
			fmt.Printf("  To:          %v\n", pktNet.DestinationAddress())
			fmt.Printf("  Payload Len: %d\n", payload.Len())
			fmt.Printf("  Payload:     % x\n", payload)
			if pkt.TransportProtocolNumber == header.ICMPv6ProtocolNumber {
				icmp := header.ICMPv6(thbuf)
				fmt.Printf("  ICMP:\n")
				fmt.Printf("    Type: %d\n", icmp.Type())
				fmt.Printf("    Code: %d\n", icmp.Code())
			}
			if pkt.TransportProtocolNumber == header.TCPProtocolNumber {
				tcpHdr := header.TCP(thbuf)
				fmt.Printf("  TCP:\n")
				fmt.Printf("    Source Port: %d\n", tcpHdr.SourcePort())
				fmt.Printf("    Dest Port:   %d\n", tcpHdr.DestinationPort())
				fmt.Printf("    Flags:       %v\n", tcpHdr.Flags())
			}
			pkt.DecRef()
		}
	}()
	go func() {
		payload := "hello"
		for {
			buf := buffer.NewView(header.IPv6MinimumSize + header.UDPMinimumSize + len(payload))
			copy(buf[len(buf)-len(payload):], payload)

			ip := header.IPv6(buf)
			ip.Encode(&header.IPv6Fields{
				PayloadLength:     uint16(header.UDPMinimumSize + len(payload)),
				TransportProtocol: udp.ProtocolNumber,
				HopLimit:          30,
				SrcAddr:           remoteAddr,
				DstAddr:           myAddr,
			})

			u := header.UDP(buf[header.IPv6MinimumSize:])
			u.Encode(&header.UDPFields{
				SrcPort: 1234,
				DstPort: 1234,
				Length:  uint16(header.UDPMinimumSize + len(payload)),
			})

			// Calculate the UDP pseudo-header checksum.
			xsum := header.PseudoHeaderChecksum(udp.ProtocolNumber, ip.SourceAddress(), ip.DestinationAddress(), uint16(len(u)))

			// Calculate the UDP checksum and set it.
			xsum = header.Checksum([]byte(payload), xsum)
			u.SetChecksum(^u.CalculateChecksum(xsum))

			// Inject packet.
			pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{
				Data: buf.ToVectorisedView(),
			})
			ep.InjectInbound(ipv6.ProtocolNumber, pkt)

			time.Sleep(2 * time.Second)
		}
	}()

	outsideFA := tcpip.FullAddress{
		NIC:  1,
		Addr: remoteAddr,
		Port: 123,
	}
	c, err = gonet.DialTCP(s, outsideFA, ipv6.ProtocolNumber)
	if err != nil {
		return err
	}

	time.Sleep(1 * time.Second)

	return nil
}

func main() {
	err := NetTest()
	if err != nil {
		fmt.Printf("Error: %s\n", err)
	}
}