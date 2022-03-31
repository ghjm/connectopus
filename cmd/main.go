package main

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/pkg/backends/backend_dtls"
	"github.com/ghjm/connectopus/pkg/netopus"
	"net"
	"os"
)

func run(ctx context.Context) error {
	nl, err := netopus.NewNetopus(ctx, net.ParseIP("FD00::1"))
	if err != nil {
		return err
	}
	err = backend_dtls.RunListener(ctx, nl, 4444)
	if err != nil {
		return err
	}
	nd, err := netopus.NewNetopus(ctx, net.ParseIP("FD00::2"))
	if err != nil {
		return err
	}
	err = backend_dtls.RunDialer(ctx, nd, net.ParseIP("127.0.0.1"), 4444)
	if err != nil {
		return err
	}
	return nil
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := run(ctx)
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		os.Exit(1)
	}
	select{}
}
