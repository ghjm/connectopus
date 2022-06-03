//go:build !linux

package netns

import (
	"context"
	"fmt"
	"net"
)

func New(ctx context.Context, addr net.IP) (*Link, error) {
	return nil, fmt.Errorf("not implemented")
}

func (ns *Link) SendPacket(packet []byte) error {
	return fmt.Errorf("not implemented")
}

func (ns *Link) PID() int {
	return ns.pid
}
