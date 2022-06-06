//go:build !linux

package tun

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/pkg/links"
	"net"
)

// New is unavailable on non-Linux platforms
func New(ctx context.Context, name string, tunAddr net.IP, subnet *net.IPNet) (links.Link, error) {
	return nil, fmt.Errorf("only implemented on Linux")
}
