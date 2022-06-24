//go:build linux

package netstack

import (
	"context"
	"net"
)

func NewStackDefault(ctx context.Context, addr net.IP, mtu uint16) (NetStack, error) {
	return NewStackFdbased(ctx, addr, mtu)
}

var stackBuilders = []NewStackFunc{
	NewStackChannel,
	NewStackFdbased,
}
