//go:build !linux

package tun

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/pkg/x/chanreader"
	"net"
)

type Link struct {
	chanreader.Publisher
}

// New is unavailable on non-Linux platforms
func New(ctx context.Context, name string, tunAddr net.IP, subnet *net.IPNet) (*Link, error) {
	return nil, fmt.Errorf("only implemented on Linux")
}

func (l *Link) SendPacket(packet []byte) error {
	return fmt.Errorf("only implemented on Linux")
}
