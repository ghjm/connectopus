package services

import (
	"bufio"
	"context"
	"github.com/ghjm/connectopus/pkg/config"
	"github.com/ghjm/connectopus/pkg/netstack"
	"github.com/google/shlex"
	log "github.com/sirupsen/logrus"
	"io"
	"net"
	"os/exec"
	"time"
)

func runCommand(ctx context.Context, conn net.Conn, args []string) error {
	defer func() {
		_ = conn.Close()
	}()
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

func acceptLoop(ctx context.Context, li net.Listener, args []string) error {
	var tempDelay time.Duration
	for {
		conn, err := li.Accept()
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			log.Warnf("accept error: %s; retrying in %v", err, tempDelay)
			if tempDelay == 0 {
				tempDelay = 5 * time.Millisecond
			} else {
				tempDelay *= 2
			}
			if max := 1 * time.Second; tempDelay > max {
				tempDelay = max
			}
			time.Sleep(tempDelay)
			continue
		}
		tempDelay = 0
		go func() {
			err = runCommand(ctx, conn, args)
			if err != nil {
				log.Warnf("service command error: %s", err)
			}
		}()
	}
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
		err := acceptLoop(ctx, li, args)
		if err != nil {
			log.Errorf("accept error: %s", err)
		}
	}()
	return li.Addr(), nil
}
