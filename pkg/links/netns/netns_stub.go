//go:build !linux

package netns

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/pkg/x/packet_publisher"
	"net"
)

type Link struct {
	packet_publisher.Publisher
}

func New(ctx context.Context, addr net.IP) (*Link, error) {
	return nil, fmt.Errorf("not implemented")
}

func (ns *Link) SendPacket(packet []byte) error {
	return fmt.Errorf("not implemented")
}

func (ns *Link) PID() int {
	return 0
}

func RunShim(fd int, tunif string, addr string) error {
	return fmt.Errorf("not implemented")
}
