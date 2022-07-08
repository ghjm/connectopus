//go:build linux

package netns

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/pkg/x/checkroot"
	"github.com/ghjm/connectopus/pkg/x/syncro"
	"go.uber.org/goleak"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"net"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"
)

func TestAsRootNetns(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if !checkroot.CheckRoot() && !checkroot.CheckNetAdmin() {
		fmt.Printf("Skipping network namespace tests due to lack of permissions\n")
		return
	}

	addr := net.ParseIP("FD02:3517:34dd:65f1:2bc8:7ae7:8c81:1")
	remote := net.ParseIP("FD02:3517:34dd:65f1:2bc8:7ae7:8c81:2")
	ns, err := New(ctx, addr, WithShimBin("../../../connectopus"))
	if err != nil {
		t.Fatal(err)
	}

	success := syncro.NewVar(false)
	packetChan := ns.SubscribePackets()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case pkt := <-packetChan:
				ipkt := header.IPv6(pkt)
				proto := ipkt.TransportProtocol()
				if proto == header.ICMPv6ProtocolNumber {
					icmpPkt := header.ICMPv6(ipkt.Payload())
					if icmpPkt.Type() == header.ICMPv6EchoRequest {
						success.Set(true)
						cancel()
						return
					}
				}
			}
		}
	}()

	command := exec.CommandContext(ctx, "nsenter", "--preserve-credentials", "--user", "--mount", "--net", "--uts",
		"-t", strconv.Itoa(ns.PID()), "ping6", "-c", "1", remote.String())
	command.Stdin = nil
	command.Stdout = os.Stdout
	command.Stderr = os.Stdout
	err = command.Start()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		err = command.Wait()
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			t.Error(err)
		}
	}()

	<-ctx.Done()
	if ctx.Err() != context.Canceled {
		t.Fatal(ctx.Err())
	}
	if !success.Get() {
		t.Fatal("packet not received")
	}

}
