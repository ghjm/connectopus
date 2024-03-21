//go:build linux

package netns

import (
	"bufio"
	"context"
	"fmt"
	"github.com/ghjm/golib/pkg/chanreader"
	"github.com/ghjm/golib/pkg/exit_handler"
	"github.com/ghjm/golib/pkg/file_cleaner"
	"github.com/ghjm/golib/pkg/syncro"
	"github.com/google/shlex"
	log "github.com/sirupsen/logrus"
	"github.com/songgao/water"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var linkReadyMessage = "netns_shim link ready\n"

type newParams struct {
	mtu     uint16
	shimBin string
}

// Link implements link.Link for a local private network namespace
type Link struct {
	chanreader.Publisher
	shimFd int
	pid    int
	mtu    uint16
}

// New creates a new Linux network namespace based network stack
func New(ctx context.Context, addr net.IP, domain string, dnsServer string, mods ...func(*newParams)) (*Link, error) {
	params := &newParams{
		mtu: 1500,
	}
	for _, mod := range mods {
		mod(params)
	}

	ns := &Link{
		mtu: params.mtu,
	}

	// Create the socketpair
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_SEQPACKET, 0)
	if err != nil {
		return nil, err
	}
	ns.shimFd = fds[0]

	// Run the command
	myExec := params.shimBin
	if myExec == "" {
		myExec, err = os.Executable()
		if err != nil {
			return nil, err
		}
	}
	cmd := exec.CommandContext(ctx, myExec, "netns_shim", "--fd", "3", "--tunif", "nstun",
		"--addr", fmt.Sprintf("%s/128", addr.String()), "--mtu", fmt.Sprintf("%d", params.mtu),
		"--domain", domain, "--dns-server", dnsServer)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Unshareflags: syscall.CLONE_NEWNS | syscall.CLONE_NEWUSER | syscall.CLONE_NEWNET | syscall.CLONE_NEWUTS,
		UidMappings:  []syscall.SysProcIDMap{{ContainerID: 0, HostID: unix.Getuid(), Size: 1}},
	}
	cmd.Stdin = nil
	var stdout io.ReadCloser
	stdout, err = cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
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

	exitFunc := exit_handler.AddExitFunc(func() {
		_ = cmd.Process.Kill()
		_ = stdout.Close()
		_ = stderr.Close()
	})

	// Kill the process if our context is cancelled
	go func() {
		<-ctx.Done()
		exitFunc()
	}()

	// Wait for the command (needed to free resources)
	go func() {
		_ = cmd.Wait()
	}()

	// Stdout handling
	chanReady := make(chan struct{})
	go func() {
		closed := false
		sr := bufio.NewReader(stdout)
		for {
			s, rerr := sr.ReadString('\n')
			if rerr != nil {
				return
			}
			if !closed && s == linkReadyMessage {
				log.Debugf("netns %s initialized", addr.String())
				closed = true
				close(chanReady)
			} else {
				log.Debugf("netns_shim extra ouptut: %s", s)
			}
		}
	}()

	// Stderr handling
	go func() {
		sr := bufio.NewReader(stderr)
		for {
			s, rerr := sr.ReadString('\n')
			if rerr != nil {
				return
			}
			log.Errorf("netns_shim error: %s", s)
		}
	}()

	// Publish incoming packets to interested receivers
	ns.Publisher = *chanreader.NewPublisher(ctx, os.NewFile(uintptr(ns.shimFd), "socket"),
		chanreader.WithBufferSize(1500))

	// Try not to return until the link is actually ready
	select {
	case <-ctx.Done():
	case <-chanReady:
	case <-time.After(time.Second):
		log.Warnf("netns_shim slow to initialize")
	}
	return ns, nil
}

// WithShimBin sets the path to the netns_shim binary.  Used for testing - normal users do not need this.
func WithShimBin(shimBin string) func(*newParams) {
	return func(p *newParams) {
		p.shimBin = shimBin
	}
}

// WithMTU sets the MTU for the tunnel interface in the namespace.
func WithMTU(mtu uint16) func(*newParams) {
	return func(p *newParams) {
		p.mtu = mtu
	}
}

func (ns *Link) SendPacket(packet []byte) error {
	_, err := syscall.Write(ns.shimFd, packet)
	return err
}

func (ns *Link) PID() int {
	return ns.pid
}

func (ns *Link) MTU() uint16 {
	return ns.mtu
}

func (ns *Link) RunInNamespace(ctx context.Context, command string, prep func(*exec.Cmd) error) (*exec.Cmd, error) {
	args, err := shlex.Split(command)
	if err != nil {
		return nil, fmt.Errorf("error splitting command args: %w", err)
	}
	args = append([]string{"--preserve-credentials", "--user", "--mount", "--net",
		"--uts", "-t", strconv.Itoa(ns.pid)}, args...)
	cmd := exec.CommandContext(ctx, "nsenter", args...)
	if prep != nil {
		err = prep(cmd)
		if err != nil {
			return nil, err
		}
	}
	err = cmd.Start()
	if err != nil {
		return nil, fmt.Errorf("error running command: %w", err)
	}
	return cmd, nil
}

func RunShim(fd int, tunif string, mtu uint16, addr string, domain string, dnsServer string) error {
	// Bring up the lo interface
	lo, err := netlink.LinkByName("lo")
	if err != nil {
		return fmt.Errorf("error opening lo: %w", err)
	}
	err = netlink.LinkSetUp(lo)
	if err != nil {
		return fmt.Errorf("error bringing up lo: %w", err)
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
		return fmt.Errorf("error creating tun interface: %w", err)
	}

	// Set the tun interface address
	var tunLink netlink.Link
	tunLink, err = netlink.LinkByName(tunif)
	if err != nil {
		return fmt.Errorf("error opening tun interface: %w", err)
	}
	var tunAddr *netlink.Addr
	tunAddr, err = netlink.ParseAddr(addr)
	if err != nil {
		return fmt.Errorf("error parsing IP address: %w", err)
	}
	err = netlink.AddrAdd(tunLink, tunAddr)
	if err != nil {
		return fmt.Errorf("error adding IP address to tun interface: %w", err)
	}

	// Set the tun MTU
	err = netlink.LinkSetMTU(tunLink, int(mtu))
	if err != nil {
		return fmt.Errorf("error setting tun interface MTU: %w", err)
	}

	// Bring up the tun interface
	err = netlink.LinkSetUp(tunLink)
	if err != nil {
		return fmt.Errorf("error bringing up tun: %w", err)
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
		return fmt.Errorf("error adding IPv4 route to tun interface: %w", err)
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
		return fmt.Errorf("error adding IPv6 route to tun interface: %w", err)
	}

	// Create resolv.conf
	var rf *os.File
	rf, err = os.CreateTemp("", "connectopus-resolv-*.conf")
	if err != nil {
		return fmt.Errorf("error creating resolv.conf temp file: %w", err)
	}
	rdh := file_cleaner.DeleteOnExit(rf.Name())
	_, err = fmt.Fprintf(rf, "nameserver %s\n", dnsServer)
	if err != nil {
		return fmt.Errorf("error writing resolv.conf temp file: %w", err)
	}
	_, err = fmt.Fprintf(rf, "search %s\n", domain)
	if err != nil {
		return fmt.Errorf("error writing resolv.conf temp file: %w", err)
	}
	err = rf.Close()
	if err != nil {
		return fmt.Errorf("error closing resolv.conf temp file: %w", err)
	}
	err = syscall.Mount(rf.Name(), "/etc/resolv.conf", "", syscall.MS_BIND, "")
	if err != nil {
		return fmt.Errorf("error binding resolv.conf: %w", err)
	}

	// Create nsswitch.conf
	var origNsf []byte
	origNsf, err = os.ReadFile("/etc/nsswitch.conf")
	if err != nil {
		return fmt.Errorf("error reading nsswitch.conf: %w", err)
	}
	var nsf *os.File
	nsf, err = os.CreateTemp("", "connectopus-nsswitch-*.conf")
	if err != nil {
		return fmt.Errorf("error creating nsswitch.conf temp file: %w", err)
	}
	ndh := file_cleaner.DeleteOnExit(nsf.Name())
	for _, line := range strings.Split(string(origNsf), "\n") {
		if strings.HasPrefix(strings.TrimLeft(line, " "), "hosts:") {
			line = "hosts: files dns"
		}
		_, err = fmt.Fprintf(nsf, "%s\n", line)
		if err != nil {
			return fmt.Errorf("error writing resolv.conf temp file: %w", err)
		}
	}
	err = nsf.Close()
	if err != nil {
		return fmt.Errorf("error closing nsswitch.conf temp file: %w", err)
	}
	err = syscall.Mount(nsf.Name(), "/etc/nsswitch.conf", "", syscall.MS_BIND, "")
	if err != nil {
		return fmt.Errorf("error binding nsswitch.conf: %w", err)
	}

	// Notify that the link is ready
	_, _ = fmt.Print(linkReadyMessage)

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
	_ = rdh.DeleteNow()
	_ = ndh.DeleteNow()
	return err
}
