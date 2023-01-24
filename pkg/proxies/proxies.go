package proxies

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/pkg/config"
	"github.com/ghjm/connectopus/pkg/proto"
	"strings"
)

type candidate struct {
	node config.Node // node to connect via
	proxy config.Proxy // external address to connect to
}

func getCandidates(cfg *config.Config, proxy config.Proxy) ([]candidate, error) {
	var candidates []candidate
	for _, c := range cfg.Nodes {
		for _, p := range c.Proxies {
			if p.Kind == "tcp-outbound" && p.Label == proxy.Label {
				candidates = append(candidates, candidate{c, p})
			}
		}
	}
	return candidates, nil
}

func RunProxy(ctx context.Context, n proto.Netopus, proxy config.Proxy) error {
	switch strings.ToLower(proxy.Kind) {
	case "tcp-inbound":
		return runProxyTCPInbound(ctx, n, proxy)
	case "tcp-outbound":
		return runProxyTCPOutbound(ctx, n, proxy)
	case "udp-inbound":
		return runProxyUDPInbound(ctx, n, proxy)
	case "udp-outbound":
		return runProxyUDPOutbound(ctx, n, proxy)
	}
	return fmt.Errorf("invalid proxy type")
}
