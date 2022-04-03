package netstack

import (
	"context"
	"fmt"
	"go.uber.org/goleak"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv6"
	"gvisor.dev/gvisor/pkg/tcpip/transport/udp"
	"io"
	"io/ioutil"
	"net"
	"testing"
	"time"
)

var testStr = "Hello, world!"

func TestNetstack(t *testing.T) {
	defer goleak.VerifyNone(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	localIP := net.ParseIP("FD00::1")
	ns := NewStack(ctx, localIP)

	// Start TCP listener
	tcpFA := tcpip.FullAddress{
		NIC:  1,
		Addr: tcpip.Address(localIP),
		Port: 1234,
	}
	li, err := gonet.ListenTCP(ns.Stack, tcpFA, ipv6.ProtocolNumber)
	if err != nil {
		t.Fatalf("listen TCP error: %s", err)
	}
	go func() {
		c, err := li.Accept()
		if err != nil {
			t.Errorf("accept error: %s", err)
			return
		}
		_, _ = c.Write([]byte(testStr))
		_ = c.Close()
	}()

	// Connect using TCP
	dctx, dcancel := context.WithTimeout(ctx, time.Second)
	defer dcancel()
	c, err := gonet.DialContextTCP(dctx, ns.Stack, tcpFA, ipv6.ProtocolNumber)
	if err != nil {
		t.Fatalf("dial TCP error: %s", err)
	}
	b, err := ioutil.ReadAll(c)
	if err != nil {
		t.Fatalf("read TCP error: %s", err)
	}
	err = c.Close()
	if err != nil {
		t.Fatalf("close TCP error: %s", err)
	}
	if string(b) != testStr {
		t.Fatalf("incorrect data received: expected %s but got %s", testStr, b)
	}
}

func TestNetstackSubscribe(t *testing.T) {
	defer goleak.VerifyNone(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	localIP := net.ParseIP("FD00::1")
	remoteIP := net.ParseIP("FD00::2")
	ns := NewStack(ctx, localIP)

	gotData := false
	go func() {
		subCh := ns.SubscribePackets()
		defer ns.UnsubscribePackets(subCh)
		for {
			select {
			case <-ctx.Done():
				return
			case packet := <-subCh:
				ip := header.IPv6(packet)
				if ip.SourceAddress() != tcpip.Address(localIP) ||
					ip.DestinationAddress() != tcpip.Address(remoteIP) ||
					!ip.IsValid(len(packet)) {
					t.Errorf("incorrect IP header received")
				}
				u := header.UDP(packet[header.IPv6MinimumSize:])
				if u.SourcePort() != 1234 || u.DestinationPort() != 1234 {
					t.Errorf("incorrect UDP header received")
				}
				if string(u.Payload()) != testStr {
					t.Errorf("incorrect payload data received")
				}
				gotData = true
			}
		}
	}()

	udpConn, err := gonet.DialUDP(ns.Stack,
		&tcpip.FullAddress{
			NIC:  1,
			Addr: tcpip.Address(localIP),
			Port: 1234,
		},
		&tcpip.FullAddress{
			NIC:  1,
			Addr: tcpip.Address(remoteIP),
			Port: 1234,
		},
		ipv6.ProtocolNumber,
	)
	if err != nil {
		t.Fatalf("DialUDP error %s", err)
	}
	startTime := time.Now()
	for {
		if gotData || time.Now().Sub(startTime) > 5 * time.Second {
			break
		}
		err = udpConn.SetWriteDeadline(time.Now().Add(100 * time.Millisecond))
		if err != nil {
			t.Fatalf("SetWriteDeadLine error %s", err)
		}
		n, err := udpConn.Write([]byte(testStr))
		if err != nil {
			t.Fatalf("SetWriteDeadLine error %s", err)
		}
		if n != len(testStr) {
			t.Fatalf("expected to write %d bytes but only wrote %d", len(testStr), n)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !gotData {
		t.Fatalf("did not receive any data")
	}

}

func TestNetstackInject(t *testing.T) {
	defer goleak.VerifyNone(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	localIP := net.ParseIP("FD00::1")
	remoteIP := net.ParseIP("FD00::2")
	ns := NewStack(ctx, localIP)

	udpFA := tcpip.FullAddress{
		NIC:  1,
		Addr: tcpip.Address(localIP),
		Port: 1234,
	}
	udpConn, err := gonet.DialUDP(ns.Stack, &udpFA, nil, ipv6.ProtocolNumber)
	if err != nil {
		t.Fatalf("DialUDP error %s", err)
	}
	var gotData bool
	go func() {
		buf := make([]byte, 1500)
		for {
			n, addr, err := udpConn.ReadFrom(buf)
			if err != nil {
				if err != io.EOF {
					t.Errorf("UDP read error: %s\n", err)
				}
				return
			}
			if addr.String() != fmt.Sprintf("[%s]:%d", remoteIP.String(), 1234) {
				t.Errorf("incorrect address: expected %s but got %s", remoteIP, addr)
			}
			if string(buf[:n]) != testStr {
				t.Errorf("incorrect payload: expected %s but got %s", testStr, buf[:n])
			}
			gotData = true
		}
	}()

	// Construct and inject a UDP packet
	packet := make([]byte, header.IPv6MinimumSize + header.UDPMinimumSize + len(testStr))
	copy(packet[header.IPv6MinimumSize + header.UDPMinimumSize:], testStr)
	ip := header.IPv6(packet)
	ip.Encode(&header.IPv6Fields{
		PayloadLength:     uint16(header.UDPMinimumSize + len(testStr)),
		TransportProtocol: udp.ProtocolNumber,
		HopLimit:          30,
		SrcAddr:           tcpip.Address(remoteIP),
		DstAddr:           tcpip.Address(localIP),
	})
	u := header.UDP(packet[header.IPv6MinimumSize:])
	u.Encode(&header.UDPFields{
		SrcPort: 1234,
		DstPort: 1234,
		Length:  uint16(header.UDPMinimumSize + len(testStr)),
	})
	xsum := header.PseudoHeaderChecksum(udp.ProtocolNumber, ip.SourceAddress(), ip.DestinationAddress(), uint16(len(u)))
	xsum = header.Checksum([]byte(testStr), xsum)
	u.SetChecksum(^u.CalculateChecksum(xsum))

	ns.InjectPacket(packet)

	startTime := time.Now()
	for {
		if gotData || time.Now().Sub(startTime) > 5 * time.Second {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !gotData {
		t.Fatalf("did not receive any data")
	}
}
