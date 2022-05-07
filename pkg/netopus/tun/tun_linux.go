//go:build linux

package tun

import (
	"context"
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/songgao/water"
	"github.com/vishvananda/netlink"
	"io"
	"net"
)

type link struct {
	ctx               context.Context
	tunRWC            io.ReadWriteCloser
	inboundPacketFunc func([]byte) error
}

// NewLink returns a new tun.Link.  inboundPacketFunc will be called when a packet arrives from the kernel.
func NewLink(ctx context.Context, name string, tunAddr net.IP, npAddr net.IP,
	subnet *net.IPNet, inboundPacketFunc func([]byte) error) (Link, error) {
	l := &link{
		ctx:               ctx,
		inboundPacketFunc: inboundPacketFunc,
	}
	tunIf, err := water.New(water.Config{
		DeviceType: water.TUN,
		PlatformSpecificParams: water.PlatformSpecificParams{
			Name: name,
		},
	})
	l.tunRWC = tunIf
	if err != nil {
		return nil, err
	}
	var nl netlink.Link
	nl, err = netlink.LinkByName(tunIf.Name())
	if err != nil {
		return nil, fmt.Errorf("error accessing link for tun device: %s", err)
	}
	err = netlink.AddrAdd(nl, &netlink.Addr{
		IPNet: &net.IPNet{
			IP:   tunAddr,
			Mask: net.CIDRMask(8*net.IPv6len, 8*net.IPv6len),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("error setting tun device address: %s", err)
	}
	err = netlink.LinkSetUp(nl)
	if err != nil {
		return nil, fmt.Errorf("error activating tun device: %s", err)
	}
	route := &netlink.Route{
		LinkIndex: nl.Attrs().Index,
		Scope:     netlink.SCOPE_UNIVERSE,
		Src:       tunAddr,
		Dst:       subnet,
	}
	err = netlink.RouteAdd(route)
	if err != nil {
		return nil, fmt.Errorf("error adding route to tun device: %s", err)
	}
	go func() {
		<-ctx.Done()
		_ = l.tunRWC.Close()
	}()
	go l.inboundLoop()
	return l, nil
}

func (l *link) inboundLoop() {
	for {
		buf := make([]byte, 1500)
		n, err := l.tunRWC.Read(buf)
		if err != nil {
			if l.ctx.Err() == nil {
				log.Warnf("tun device read error: %s", err)
			}
			return
		}
		buf = buf[:n]
		err = l.inboundPacketFunc(buf)
		if err != nil {
			log.Warnf("tun inbound packet error: %s", err)
		}
	}
}

func (l *link) SendPacket(packet []byte) error {
	n, err := l.tunRWC.Write(packet)
	if err != nil {
		return err
	}
	if n != len(packet) {
		return fmt.Errorf("tun device only wrote %d bytes of %d", n, len(packet))
	}
	return nil
}
