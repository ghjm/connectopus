//go:build !linux

package netstack

import (
	"context"
	"net"
)

func NewStackDefault(ctx context.Context, subnet *net.IPNet, addr net.IP) (NetStack, error) {
	return NewStackChannel(ctx, subnet, addr)
}

var stackBuilders = []NewStackFunc{
	NewStackChannel,
}
