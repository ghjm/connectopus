//go:build !linux

package netns

import (
	"context"
	"fmt"
	"net"
)

func New(ctx context.Context, addr net.IP) (*Link, error) {
	return nil, fmt.Errorf("only implemented on Linux")
}
