//go:build !linux

package tun

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/pkg/x/chanreader"
	"github.com/songgao/water"
	"net"
)

type Link struct {
	chanreader.Publisher
}

func SetupLink(deviceName string, tunAddr net.IP, subnet *net.IPNet, mtu uint16, opts ...func()) (*water.Interface, error) {
	return nil, fmt.Errorf("not implemented")
}

func New(ctx context.Context, deviceName string, tunAddr net.IP, subnet *net.IPNet, mtu uint16) (*Link, error) {
	return nil, fmt.Errorf("not implemented")
}

func WithUidGid(uid uint, gid uint) func() {
	return nil
}

func WithPersist() func() {
	return nil
}

func WithCheck() func() {
	return nil
}

func (l *Link) SendPacket(packet []byte) error {
	return fmt.Errorf("not implemented")
}

func (l *Link) MTU() uint16 {
	return 1500
}
