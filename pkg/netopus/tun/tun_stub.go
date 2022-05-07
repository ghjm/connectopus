//go:build !linux

package tun

import (
	"context"
	"fmt"
	"net"
)

// NewLink is unavailable on non-Linux platforms
func NewLink(ctx context.Context, name string, tunAddr net.IP, npAddr net.IP,
	subnet *net.IPNet, inboundPacketFunc func([]byte) error) (Link, error) {
	return nil, fmt.Errorf("only implemented on Linux")
}
