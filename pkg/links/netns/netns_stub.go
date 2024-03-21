//go:build !linux

package netns

import (
	"context"
	"fmt"
	"github.com/ghjm/golib/pkg/chanreader"
	"net"
	"os/exec"
)

type Link struct {
	chanreader.Publisher
}

func New(ctx context.Context, addr net.IP, domain string, dnsServer string, mods ...func()) (*Link, error) {
	return nil, fmt.Errorf("not implemented")
}

func WithMTU(mtu uint16) func() {
	return nil
}

func (ns *Link) SendPacket(packet []byte) error {
	return fmt.Errorf("not implemented")
}

func (ns *Link) PID() int {
	return 0
}

func (ns *Link) MTU() uint16 {
	return 0
}

func (ns *Link) RunInNamespace(ctx context.Context, command string, prep func(*exec.Cmd) error) (*exec.Cmd, error) {
	return nil, fmt.Errorf("not implemented")
}

func RunShim(fd int, tunif string, mtu uint16, addr string, domain string, dnsServer string) error {
	return fmt.Errorf("not implemented")
}
