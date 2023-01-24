package proxies

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/pkg/config"
	"github.com/ghjm/connectopus/pkg/proto"
	"github.com/ghjm/connectopus/pkg/x/accept_loop"
	"github.com/ghjm/connectopus/pkg/x/bridge"
	"github.com/ghjm/connectopus/pkg/x/cmrand"
	log "github.com/sirupsen/logrus"
	"math"
	"net"
	"strconv"
	"strings"
)

func runProxyTCPInbound(ctx context.Context, n proto.Netopus, proxy config.Proxy) error {
	if strings.ToLower(proxy.Kind) != "tcp-inbound" {
		return fmt.Errorf("invalid proxy type")
	}

	cfg, err := config.ParseConfig(n.GetConfig())
	if err != nil {
		return err
	}

	candidates, err := getCandidates(cfg, proxy)
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		return fmt.Errorf("no destination for proxy %s", proxy.Label)
	}

	host, port, err := net.SplitHostPort(proxy.Address)
	if err != nil {
		return fmt.Errorf("error parsing proxy address: %w", err)
	}
	hostIP := proto.ParseIP(host)
	portNo, err := strconv.Atoi(port)
	if err != nil {
		return fmt.Errorf("error parsing proxy port: %w", err)
	}
	var li net.Listener
	if hostIP.Equal(n.Addr()) {
		li, err = n.ListenTCP(uint16(portNo))
	} else {
		li, err = net.ListenTCP("tcp", &net.TCPAddr{
			IP:   net.IP(hostIP),
			Port: portNo,
		})
	}
	if err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		_ = li.Close()
	}()
	go func() {
		accept_loop.AcceptLoop(ctx, li, func(ctx context.Context, conn net.Conn) {
			// Decide which candidate to connect to
			status := n.Status()
			var bestCost float32 = math.MaxFloat32
			for _, c := range candidates {
				cost, ok := status.RouterCosts[c.node.Address.String()]
				if ok && cost < bestCost {
					bestCost = cost
				}
			}
			viable := make([]candidate, 0, len(candidates))
			for _, c := range candidates {
				cost, ok := status.RouterCosts[c.node.Address.String()]
				if ok && cost <= bestCost {
					viable = append(viable, c)
				}
			}
			if len(viable) == 0 {
				log.Errorf("no viable proxy receivers for proxy label %s", proxy.Label)
				return
			}
			remote := viable[cmrand.Rand().Intn(len(viable))]

			// Connect to the candidate via OOB
			oc, err := n.DialOOB(ctx, proto.OOBAddr{Host: remote.node.Address, Port: 7769})
			if err != nil {
				log.Errorf("proxy outbound connection failed: %s", err)
				return
			}
			err = bridge.RunBridge(conn, oc)
			if err != nil {
				log.Errorf("proxy bridge error: %s", err)
			}
		})
	}()
	return nil
}

func runProxyTCPOutbound(ctx context.Context, n proto.Netopus, proxy config.Proxy) error {
	if strings.ToLower(proxy.Kind) != "tcp-outbound" {
		return fmt.Errorf("invalid proxy type")
	}
	return fmt.Errorf("not implemented")
}
