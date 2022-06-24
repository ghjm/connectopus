//go:build linux

package tun

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/pkg/x/chanreader"
	"github.com/songgao/water"
	"github.com/vishvananda/netlink"
	"io"
	"net"
)

type Link struct {
	chanreader.Publisher
	ctx    context.Context
	tunRWC io.ReadWriteCloser
}

// New returns a link.Link connected to a newly created Linux tun/tap device.
func New(ctx context.Context, deviceName string, tunAddr net.IP, subnet *net.IPNet) (*Link, error) {
	persistTun := true
	nl, err := netlink.LinkByName(deviceName)
	if _, ok := err.(netlink.LinkNotFoundError); ok {
		persistTun = false
	} else if err != nil {
		return nil, fmt.Errorf("error accessing tun device: %s", err)
	}
	var tunIf *water.Interface
	tunIf, err = water.New(water.Config{
		DeviceType: water.TUN,
		PlatformSpecificParams: water.PlatformSpecificParams{
			Name:    deviceName,
			Persist: persistTun,
		},
	})
	if err != nil {
		return nil, err
	}
	if nl == nil {
		nl, err = netlink.LinkByName(deviceName)
		if err != nil {
			return nil, fmt.Errorf("error accessing tun device: %s", err)
		}

	}
	l := &Link{
		ctx:    ctx,
		tunRWC: tunIf,
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
	var routes []netlink.Route
	routes, err = netlink.RouteList(nl, netlink.FAMILY_V6)
	if err != nil {
		return nil, fmt.Errorf("error listing routes: %s", err)
	}
	found = false
	for _, route := range routes {
		if route.Scope == netlink.SCOPE_UNIVERSE &&
			route.Src.Equal(tunAddr) &&
			route.Dst.String() == subnet.String() {
			found = true
			break
		}
	}
	if !found {
		route := netlink.Route{
			LinkIndex: nl.Attrs().Index,
			Scope:     netlink.SCOPE_UNIVERSE,
			Src:       tunAddr,
			Dst:       subnet,
		}
		err = netlink.RouteAdd(&route)
		if err != nil {
			return nil, fmt.Errorf("error adding route to tun device: %s", err)
		}
	}

	l.Publisher = *chanreader.NewPublisher(ctx, tunIf, chanreader.WithBufferSize(1500))

	return l, nil
}

func (l *Link) SendPacket(packet []byte) error {
	n, err := l.tunRWC.Write(packet)
	if err != nil {
		return err
	}
	if n != len(packet) {
		return fmt.Errorf("tun device only wrote %d bytes of %d", n, len(packet))
	}
	return nil
}
