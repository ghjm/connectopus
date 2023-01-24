package proxies

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/pkg/config"
	"github.com/ghjm/connectopus/pkg/proto"
	"strings"
)

func runProxyUDPInbound(ctx context.Context, n proto.Netopus, proxy config.Proxy) error {
	if strings.ToLower(proxy.Kind) != "udp-inbound" {
		return fmt.Errorf("invalid proxy type")
	}
	//addr, err := net.ResolveUDPAddr("udp", proxy.Address)
	//if err != nil {
	//	return err
	//}
	//conn, err := net.ListenUDP("udp", addr)
	//if err != nil {
	//	return err
	//}
	//go func() {
	//	<-ctx.Done()
	//	_ = conn.Close()
	//}()
	//buf := make([]byte, 4096)
	//for ctx.Err() == nil {
	//	n, addr, err := conn.ReadFromUDP(buf)
	//}
	return fmt.Errorf("not implemented")
}

func runProxyUDPOutbound(ctx context.Context, n proto.Netopus, proxy config.Proxy) error {
	if strings.ToLower(proxy.Kind) != "udp-outbound" {
		return fmt.Errorf("invalid proxy type")
	}
	return fmt.Errorf("not implemented")
}
