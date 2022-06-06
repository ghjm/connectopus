//go:build linux

package tun

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/pkg/x/chanreader"
	"github.com/ghjm/connectopus/pkg/x/packet_publisher"
	"github.com/songgao/water"
	"github.com/vishvananda/netlink"
	"net"
)

// New returns a link.Link connected to a newly created Linux tun/tap device.
func New(ctx context.Context, name string, tunAddr net.IP, subnet *net.IPNet) (*Link, error) {
	persistTun := true
	nl, err := netlink.LinkByName(name)
	if _, ok := err.(netlink.LinkNotFoundError); ok {
		persistTun = false
	} else if err != nil {
		return nil, fmt.Errorf("error accessing tun device: %s", err)
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
	if nl == nil {
		nl, err = netlink.LinkByName(name)
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
	route := netlink.Route{
		LinkIndex: nl.Attrs().Index,
		Scope:     netlink.SCOPE_UNIVERSE,
		Src:       tunAddr,
		Dst:       subnet,
	}
	err = netlink.RouteAdd(&route)
	if err != nil { // && !errors.Is(syscall.EEXIST, err){
		return nil, fmt.Errorf("error adding route to tun device: %s", err)
	}

	l.Publisher = *packet_publisher.New(ctx, tunIf, chanreader.WithBufferSize(1500))

	return l, nil
}
