//go:build !linux

package netstack

import (
	"context"
	"net"
)

func NewStackDefault(ctx context.Context, addr net.IPNet) (NetStack, error) {
	return NewStackChannel(ctx, addr)
}

var stackBuilders = []NewStackFunc{
	NewStackChannel,
}
