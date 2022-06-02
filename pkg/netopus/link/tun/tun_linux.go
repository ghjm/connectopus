//go:build linux

package tun

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/pkg/netopus/link"
	"github.com/ghjm/connectopus/pkg/netopus/link/packet_publisher"
	"github.com/songgao/water"
	"github.com/vishvananda/netlink"
	"io"
	"net"
)

type tunLink struct {
	packet_publisher.Publisher
	ctx    context.Context
	tunRWC io.ReadWriteCloser
}

// New returns a link.Link connected to a newly created Linux tun/tap device.
func New(ctx context.Context, name string, tunAddr net.IP, subnet *net.IPNet) (link.Link, error) {
	persistTun := true
	nl, err := netlink.LinkByName(name)
	if _, ok := err.(netlink.LinkNotFoundError); ok {
		persistTun = false
	} else if err != nil {
		return nil, fmt.Errorf("error accessing tunLink for tun device: %s", err)
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
	l := &tunLink{
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

	l.Publisher = *packet_publisher.New(ctx, tunIf, 1500)

	return l, nil
}

func (l *tunLink) SendPacket(packet []byte) error {
	n, err := l.tunRWC.Write(packet)
	if err != nil {
		return err
	}
	if n != len(packet) {
		return fmt.Errorf("tun device only wrote %d bytes of %d", n, len(packet))
	}
	return nil
}
