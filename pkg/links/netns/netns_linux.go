//go:build linux

package netns

import (
	"bufio"
	"context"
	"fmt"
	"github.com/ghjm/connectopus/pkg/x/chanreader"
	"github.com/ghjm/connectopus/pkg/x/packet_publisher"
	"github.com/ghjm/connectopus/pkg/x/syncro"
	log "github.com/sirupsen/logrus"
	"github.com/songgao/water"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
	"io"
	"net"
	"os"
	"os/exec"
	"syscall"
)

// New creates a new Linux network namespace based network stack
func New(ctx context.Context, addr net.IP) (*Link, error) {
	ns := &Link{}

	// Create the socketpair
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_SEQPACKET, 0)
	if err != nil {
		return nil, err
	}
	ns.shimFd = fds[0]

	// Run the command
	var myExec string
	myExec, err = os.Executable()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(myExec, "netns_shim", "--fd", "3", "--tunif", "nstun",
		"--addr", fmt.Sprintf("%s/128", addr.String()))
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Unshareflags: syscall.CLONE_NEWNS | syscall.CLONE_NEWUSER | syscall.CLONE_NEWNET | syscall.CLONE_NEWUTS,
		UidMappings:  []syscall.SysProcIDMap{{ContainerID: 0, HostID: unix.Getuid(), Size: 1}},
	}
	cmd.Stdin = nil
	cmd.Stdout = nil
	var stderr io.ReadCloser
	stderr, err = cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	cmd.ExtraFiles = []*os.File{os.NewFile(uintptr(fds[1]), "socket")}
	err = cmd.Start()
	if err != nil {
		return nil, err
	}
	ns.pid = cmd.Process.Pid

	// Clean up fd we're no longer interested in
	err = syscall.Close(fds[1])
	if err != nil {
		return nil, err
	}

	// Kill the process if our context is cancelled
	go func() {
		<-ctx.Done()
		_ = cmd.Process.Kill()
		_ = stderr.Close()
	}()

	// Wait for the command (needed to free resources)
	go func() {
		_ = cmd.Wait()
	}()

	// Stderr handling
	go func() {
		sr := bufio.NewReader(stderr)
		for {
			s, rerr := sr.ReadString('\n')
			if rerr != nil {
				return
			}
			log.Warnf("netns_shim error: %s", s)
		}
	}()

	// Publish incoming packets to interested receivers
	ns.Publisher = *packet_publisher.New(ctx, os.NewFile(uintptr(ns.shimFd), "socket"),
		chanreader.WithBufferSize(1500))

	return ns, nil
}

func (ns *Link) SendPacket(packet []byte) error {
	_, err := syscall.Write(ns.shimFd, packet)
	return err
}

func (ns *Link) PID() int {
	return ns.pid
}

func RunShim(fd int, tunif string, addr string) error {

	// Bring up the lo interface
	lo, err := netlink.LinkByName("lo")
	if err != nil {
		return fmt.Errorf("error opening lo: %v", err)
	}
	err = netlink.LinkSetUp(lo)
	if err != nil {
		return fmt.Errorf("error bringing up lo: %v", err)
	}

	// Create the tun interface
	var tun *water.Interface
	tun, err = water.New(water.Config{
		DeviceType: water.TUN,
		PlatformSpecificParams: water.PlatformSpecificParams{
			Name:    tunif,
			Persist: true,
		},
	})
	if err != nil {
		return fmt.Errorf("error creating tun interface: %v", err)
	}

	// Set the tun interface address
	var tunLink netlink.Link
	tunLink, err = netlink.LinkByName(tunif)
	if err != nil {
		return fmt.Errorf("error opening tun interface: %v", err)
	}
	var tunAddr *netlink.Addr
	tunAddr, err = netlink.ParseAddr(addr)
	if err != nil {
		return fmt.Errorf("error parsing IP address: %v", err)
	}
	err = netlink.AddrAdd(tunLink, tunAddr)
	if err != nil {
		return fmt.Errorf("error adding IP address to tun interface: %v", err)
	}

	// Bring up the tun interface
	err = netlink.LinkSetUp(tunLink)
	if err != nil {
		return fmt.Errorf("error bringing up tun: %v", err)
	}

	// Set the tun interface IPv4 default route
	err = netlink.RouteAdd(&netlink.Route{
		LinkIndex: tunLink.Attrs().Index,
		Scope:     netlink.SCOPE_UNIVERSE,
		Dst: &net.IPNet{
			IP:   net.ParseIP("0.0.0.0"),
			Mask: net.CIDRMask(0, 32),
		},
		Hoplimit: 30,
	})
	if err != nil {
		return fmt.Errorf("error adding IPv4 route to tun interface: %v", err)
	}

	// Set the tun interface IPv6 default route
	err = netlink.RouteAdd(&netlink.Route{
		LinkIndex: tunLink.Attrs().Index,
		Scope:     netlink.SCOPE_UNIVERSE,
		Dst: &net.IPNet{
			IP:   net.IPv6unspecified,
			Mask: net.CIDRMask(0, 128),
		},
		Hoplimit: 30,
	})
	if err != nil {
		return fmt.Errorf("error adding IPv6 route to tun interface: %v", err)
	}

	// Copy packets between the tun interface and the provided fd
	serr := syncro.Var[error]{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := os.NewFile(uintptr(fd), "conn")
	go func() {
		_, gerr := io.Copy(f, tun)
		if gerr != nil {
			serr.Set(gerr)
		}
		cancel()
	}()
	go func() {
		_, gerr := io.Copy(tun, f)
		if gerr != nil {
			serr.Set(gerr)
		}
		cancel()
	}()
	<-ctx.Done()
	err = serr.Get()
	_ = tun.Close()
	_ = f.Close()
	return err
}
