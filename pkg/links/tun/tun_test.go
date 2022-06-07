//go:build linux

package tun

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/pkg/x/checkroot"
	"github.com/ghjm/connectopus/pkg/x/syncro"
	"go.uber.org/goleak"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"math/rand"
	"net"
	"testing"
	"time"
)

func checkPacket(pkt []byte, port int, message string) bool {
	ipkt := header.IPv6(pkt)
	proto := ipkt.TransportProtocol()
	if proto == header.UDPProtocolNumber {
		udpPkt := header.UDP(ipkt.Payload())
		if udpPkt.DestinationPort() == uint16(port) {
			payload := string(udpPkt.Payload())
			if payload == message {
				return true
			}
		}
	}
	return false
}

func TestAsRootTun(t *testing.T) {
	rand.Seed(time.Now().UnixNano())
	goleak.VerifyNone(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if !checkroot.CheckRoot() && !checkroot.CheckNetAdmin() {
		fmt.Printf("Skipping tun link tests due to lack of permissions\n")
		return
	}

	testMessage := "Hello, world!\n"
	var success syncro.Var[bool]
	local := net.ParseIP("FD01:d9d9:12eb:7465:fd59:ec45:142c:1")
	remote := net.ParseIP("FD01:d9d9:12eb:7465:fd59:ec45:142c:2")
	subnet := &net.IPNet{
		IP:   net.ParseIP("FD01:d9d9:12eb:7465::"),
		Mask: net.CIDRMask(64, 8*net.IPv6len),
	}
	devName := fmt.Sprintf("tuntest%d", rand.Intn(1000))
	tt, err := New(ctx, devName, local, subnet)
	if err != nil {
		t.Fatal(err)
	}

	// Test receiving from the tun link
	recvCtx, recvCancel := context.WithCancel(ctx)
	defer recvCancel()
	success.Set(false)
	packChan := tt.SubscribePackets()
	go func() {
		for {
			select {
			case <-recvCtx.Done():
				return
			case pkt := <-packChan:
				if checkPacket(pkt, 1000, testMessage) {
					success.Set(true)
					recvCancel()
				}
			}
		}
	}()
	var uc net.Conn
	uc, err = net.Dial("udp", net.JoinHostPort(remote.String(), "1000"))
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			_, err := uc.Write([]byte(testMessage))
			if err != nil {
				t.Error(err)
			}
			select {
			case <-recvCtx.Done():
				return
			case <-time.After(100 * time.Millisecond):
			}
		}
	}()
	select {
	case <-recvCtx.Done():
	case <-time.After(time.Second):
		recvCancel()
	}
	if !success.Get() {
		t.Fatalf("incoming message not received")
	}
	_ = uc.Close()
}
