//go:build linux

package netns

import (
	"context"
	"github.com/ghjm/connectopus/pkg/netopus/link/packet_publisher"
	"github.com/ghjm/connectopus/pkg/x/chanreader"
	"net"
	"os"
	"os/exec"
	"syscall"
)

// New creates a new Linux network namespace based network stack
func New(ctx context.Context, addr net.IP) (*Link, error) {
	ns := &Link{}

	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_SEQPACKET, 0)
	if err != nil {
		return nil, err
	}
	ns.shimFd = fds[0]

	cmd := exec.Command("netns_shim/netns_shim", "-f", "3", "-t", "nstun", "-a", addr.String())
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.ExtraFiles = []*os.File{os.NewFile(uintptr(fds[1]), "socket")}
	err = cmd.Start()
	if err != nil {
		return nil, err
	}
	// _ = remotePipe.Close()

	ns.Publisher = *packet_publisher.New(ctx, os.NewFile(uintptr(ns.shimFd), "socket"),
		chanreader.WithBufferSize(1500))

	return ns, nil
}

func (ns *Link) SendPacket(packet []byte) error {
	_, err := syscall.Write(ns.shimFd, packet)
	return err
}
