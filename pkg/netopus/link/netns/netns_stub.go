//go:build !linux

package netns

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/pkg/netopus/link"
	"net"
)

func NewNetns(ctx context.Context, addr net.IP) (link.Link, error) {
	return nil, fmt.Errorf("only implemented on Linux")
}
