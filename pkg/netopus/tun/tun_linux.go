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
func NewLink(ctx context.Context, name string, tunAddr net.IP,
	subnet *net.IPNet, inboundPacketFunc func([]byte) error) (Link, error) {
	persistTun := true
	nl, err := netlink.LinkByName(name)
	if _, ok := err.(netlink.LinkNotFoundError); ok {
		persistTun = false
	} else if err != nil {
		return nil, fmt.Errorf("error accessing link for tun device: %s", err)
	}
	var tunIf *water.Interface
	tunIf, err = water.New(water.Config{
		DeviceType: water.TUN,
		PlatformSpecificParams: water.PlatformSpecificParams{
			Name:    name,
			Persist: persistTun,
		},
	})
	if err != nil {
		return nil, err
	}
	l := &link{
		ctx:               ctx,
		tunRWC:            tunIf,
		inboundPacketFunc: inboundPacketFunc,
	}
	var addrs []netlink.Addr
	addrs, err = netlink.AddrList(nl, netlink.FAMILY_V6)
	if err != nil {
		return nil, fmt.Errorf("error listing addresses of tun device: %s", err)
	}
	tunMask := net.CIDRMask(8*net.IPv6len, 8*net.IPv6len)
	found := false
	for _, addr := range addrs {
		if addr.IP.Equal(tunAddr) && addr.Mask.String() == tunMask.String() {
			found = true
			break
		}
	}
	if !found {
		err = netlink.AddrAdd(nl, &netlink.Addr{
			IPNet: &net.IPNet{
				IP:   tunAddr,
				Mask: net.CIDRMask(8*net.IPv6len, 8*net.IPv6len),
			},
		})
		if err != nil {
			return nil, fmt.Errorf("error setting tun device address: %s", err)
		}
	}
	if nl.Attrs().Flags&net.FlagUp == 0 {
		err = netlink.LinkSetUp(nl)
		if err != nil {
			return nil, fmt.Errorf("error activating tun device: %s", err)
		}
	}
	route := netlink.Route{
		LinkIndex: nl.Attrs().Index,
		Scope:     netlink.SCOPE_UNIVERSE,
		Src:       tunAddr,
		Dst:       subnet,
	}
	var routes []netlink.Route
	routes, err = netlink.RouteList(nl, netlink.FAMILY_V6)
	if err != nil {
		return nil, fmt.Errorf("error listing routes: %s", err)
	}
	found = false
	for _, r := range routes {
		if r.Dst.IP.Equal(route.Dst.IP) &&
			r.Dst.Mask.String() == route.Dst.Mask.String() &&
			r.Gw.Equal(route.Gw) &&
			r.Src.Equal(route.Src) {
			found = true
			break
		}
	}
	if !found {
		err = netlink.RouteAdd(&route)
		if err != nil {
			return nil, fmt.Errorf("error adding route to tun device: %s", err)
		}
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
