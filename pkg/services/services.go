package services

import (
	"bufio"
	"context"
	"github.com/ghjm/connectopus/pkg/config"
	"github.com/ghjm/connectopus/pkg/netstack"
	"github.com/ghjm/connectopus/pkg/x/accept_loop"
	"github.com/google/shlex"
	log "github.com/sirupsen/logrus"
	"io"
	"net"
	"os/exec"
)

func runCommand(ctx context.Context, conn net.Conn, args []string) error {
	defer func() {
		_ = conn.Close()
	}()
	//nolint: gosec // potential tainted cmd arguments
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	cmd.Stdout = conn
	var stderr io.ReadCloser
	stderr, err = cmd.StderrPipe()
	if err != nil {
		return err
	}
	err = cmd.Start()
	if err != nil {
		return err
	}
	go func() {
		_, err := io.Copy(stdin, conn)
		if err != nil {
			log.Warnf("read error in service: %s", err)
		}
		err = stdin.Close()
		if err != nil {
			log.Warnf("error closing service stdin: %s", err)
		}
	}()
	go func() {
		sr := bufio.NewReader(stderr)
		for {
			s, rerr := sr.ReadString('\n')
			if rerr != nil {
				return
			}
			log.Warnf("service error: %s", s)
		}
	}()
	err = cmd.Wait()
	if err != nil {
		return err
	}
	return nil
}

func RunService(ctx context.Context, n netstack.UserStack, service config.Service) (net.Addr, error) {
	cmd := service.Command
	args, err := shlex.Split(cmd)
	if err != nil {
		return nil, err
	}
	var li net.Listener
	li, err = n.ListenTCP(uint16(service.Port))
	if err != nil {
		return nil, err
	}
	go func() {
		<-ctx.Done()
		_ = li.Close()
	}()
	go func() {
		accept_loop.AcceptLoop(ctx, li, func(ctx context.Context, conn net.Conn) {
			err := runCommand(ctx, conn, args)
			if err != nil {
				log.Warnf("service command error: %s", err)
			}
		})
	}()
	return li.Addr(), nil
}
