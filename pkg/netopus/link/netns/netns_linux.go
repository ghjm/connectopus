//go:build linux

package netns

import (
	"bufio"
	"context"
	"fmt"
	"github.com/ghjm/connectopus/pkg/netopus/link/packet_publisher"
	"github.com/ghjm/connectopus/pkg/x/chanreader"
	log "github.com/sirupsen/logrus"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
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
	cmd := exec.Command("netns_shim/netns_shim", "-f", "3", "-t", "nstun", "-a", addr.String())
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

	// Clean up fd we're no longer interested in
	err = syscall.Close(fds[1])
	if err != nil {
		return nil, err
	}

	// Kill the process if our context is cancelled
	go func() {
		<-ctx.Done()
		_ = cmd.Process.Kill()
		_ = stdout.Close()
		_ = stderr.Close()
	}()

	// Wait for the command (needed to free resources)
	go func() {
		_ = cmd.Wait()
	}()

	// Stdout handling
	pidch := make(chan int)
	go func() {
		defer close(pidch)
		sr := bufio.NewReader(stdout)
		for {
			s, rerr := sr.ReadString('\n')
			if rerr != nil {
				return
			}
			prefixStr := "Unshared PID: "
			if strings.HasPrefix(s, prefixStr) {
				s = strings.TrimPrefix(s, prefixStr)
				s = strings.TrimSpace(s)
				pid, serr := strconv.Atoi(s)
				if serr != nil {
					return
				}
				pidch <- pid
				return
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
			log.Warnf("netns_shim error: %s", s)
		}
	}()

	// Publish incoming packets to interested receivers
	ns.Publisher = *packet_publisher.New(ctx, os.NewFile(uintptr(ns.shimFd), "socket"),
		chanreader.WithBufferSize(1500))

	// Wait for the PID from the shim
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(5 * time.Second):
		return nil, fmt.Errorf("timed out waiting for ns_shim to send pid")
	case pid := <-pidch:
		ns.pid = pid
		log.Infof("ns_shim PID is %d", pid)
	}

	return ns, nil
}

func (ns *Link) SendPacket(packet []byte) error {
	_, err := syscall.Write(ns.shimFd, packet)
	return err
}
