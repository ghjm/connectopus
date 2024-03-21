//go:build linux

package tun

import (
	"context"
	"errors"
	"fmt"
	"github.com/ghjm/golib/pkg/chanreader"
	"github.com/songgao/water"
	"github.com/vishvananda/netlink"
	"io"
	"net"
)

type Link struct {
	chanreader.Publisher
	ctx    context.Context
	tunRWC io.ReadWriteCloser
	mtu    uint16
}

type linkOptInfo struct {
	setUser bool
	uid     uint
	gid     uint
	persist bool
	check   bool
}

func SetupLink(deviceName string, tunAddr net.IP, subnet *net.IPNet, mtu uint16, opts ...func(*linkOptInfo)) (*water.Interface, error) {
	linkOpts := linkOptInfo{}
	for _, o := range opts {
		o(&linkOpts)
	}
	persistTun := true
	nl, err := netlink.LinkByName(deviceName)
	nle := netlink.LinkNotFoundError{}
	if errors.As(err, &nle) {
		if linkOpts.check {
			return nil, fmt.Errorf("tun device %s does not exist", deviceName)
		}
		persistTun = linkOpts.persist
	} else if err != nil {
		return nil, fmt.Errorf("error accessing tun device: %w", err)
	}
	var tunIf *water.Interface
	success := false
	if !linkOpts.check {
		waterCfg := water.Config{
			DeviceType: water.TUN,
			PlatformSpecificParams: water.PlatformSpecificParams{
				Name:    deviceName,
				Persist: persistTun,
			},
		}
		if linkOpts.setUser {
			waterCfg.PlatformSpecificParams.Permissions = &water.DevicePermissions{
				Owner: linkOpts.uid,
				Group: linkOpts.gid,
			}
		}
		tunIf, err = water.New(waterCfg)
		if err != nil {
			return nil, fmt.Errorf("error opening tunnel interface: %w", err)
		}
		defer func() {
			if !success {
				_ = tunIf.Close()
			}
		}()
		// populate nl if we just created the link
		if nl == nil {
			nl, err = netlink.LinkByName(deviceName)
			if err != nil {
				return nil, fmt.Errorf("error accessing tun device: %w", err)
			}
		}
	}
	var addrs []netlink.Addr
	addrs, err = netlink.AddrList(nl, netlink.FAMILY_V6)
	if err != nil {
		return nil, fmt.Errorf("error listing addresses of tun device: %w", err)
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
		if linkOpts.check {
			return nil, fmt.Errorf("tun device %s does not have address %s", deviceName, tunAddr.String())
		}
		err = netlink.AddrAdd(nl, &netlink.Addr{
			IPNet: &net.IPNet{
				IP:   tunAddr,
				Mask: net.CIDRMask(8*net.IPv6len, 8*net.IPv6len),
			},
		})
		if err != nil {
			return nil, fmt.Errorf("error setting tun device address: %w", err)
		}
	}

	if nl.Attrs().MTU != int(mtu) {
		if linkOpts.check {
			return nil, fmt.Errorf("tun device %s has MTU %d but should be %d", deviceName, nl.Attrs().MTU, mtu)
		}
		err = netlink.LinkSetMTU(nl, int(mtu))
		if err != nil {
			return nil, fmt.Errorf("error setting tun interface MTU to %d: %w", mtu, err)
		}
	}

	if nl.Attrs().Flags&net.FlagUp == 0 {
		if linkOpts.check {
			return nil, fmt.Errorf("tun device %s is down", deviceName)
		}
		err = netlink.LinkSetUp(nl)
		if err != nil {
			return nil, fmt.Errorf("error activating tun device: %w", err)
		}
	}
	var routes []netlink.Route
	routes, err = netlink.RouteList(nl, netlink.FAMILY_V6)
	if err != nil {
		return nil, fmt.Errorf("error listing routes: %w", err)
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
		if linkOpts.check {
			return nil, fmt.Errorf("tun device %s does not have a route to %s", deviceName, subnet.String())
		}
		route := netlink.Route{
			LinkIndex: nl.Attrs().Index,
			Scope:     netlink.SCOPE_UNIVERSE,
			Src:       tunAddr,
			Dst:       subnet,
		}
		err = netlink.RouteAdd(&route)
		if err != nil {
			return nil, fmt.Errorf("error adding route %s to tun device: %w", subnet.String(), err)
		}
	}

	success = true
	return tunIf, nil
}

func WithUidGid(uid uint, gid uint) func(info *linkOptInfo) {
	return func(o *linkOptInfo) {
		o.setUser = true
		o.uid = uid
		o.gid = gid
	}
}

func WithPersist() func(info *linkOptInfo) {
	return func(o *linkOptInfo) {
		o.persist = true
	}
}

func WithCheck() func(info *linkOptInfo) {
	return func(o *linkOptInfo) {
		o.check = true
	}
}

// New returns a link.Link connected to a newly created Linux tun/tap device.
func New(ctx context.Context, deviceName string, tunAddr net.IP, subnet *net.IPNet, mtu uint16) (*Link, error) {
	tunIf, err := SetupLink(deviceName, tunAddr, subnet, mtu)
	if err != nil {
		return nil, err
	}
	l := &Link{
		ctx:    ctx,
		tunRWC: tunIf,
		mtu:    mtu,
	}
	go func() {
		<-ctx.Done()
		_ = tunIf.Close()
	}()
	l.Publisher = *chanreader.NewPublisher(ctx, tunIf, chanreader.WithBufferSize(int(mtu)))
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

func (l *Link) MTU() uint16 {
	return l.mtu
}
