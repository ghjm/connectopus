//go:build !linux

package netstack

import (
	"context"
	"net"
)

func NewStackDefault(ctx context.Context, addr net.IP, mtu uint16) (NetStack, error) {
	return NewStackChannel(ctx, addr, mtu)
}

var stackBuilders = []NewStackFunc{
	NewStackChannel,
}
