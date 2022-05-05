package main

import (
	"bufio"
	"context"
	"github.com/ghjm/connectopus/pkg/config"
	"github.com/ghjm/connectopus/pkg/netopus/netstack"
	log "github.com/sirupsen/logrus"
	"net"
)

func runSession(ctx context.Context, conn net.Conn) {
	sCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		<-sCtx.Done()
		_ = conn.Close()
	}()
	r := bufio.NewReader(conn)
	for {
		if ctx.Err() != nil {
			return
		}
		str, err := r.ReadString('\n')
		if err != nil {
			return
		}
		if ctx.Err() != nil {
			return
		}
		_, err = conn.Write([]byte(str))
		if err != nil {
			return
		}
	}
}

func runServer(ctx context.Context, stack netstack.UserStack, port uint16) error {
	li, err := stack.ListenTCP(port)
	if err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		_ = li.Close()
	}()
	for {
		var conn net.Conn
		conn, err = li.Accept()
		if err != nil {
			return nil
		}
		go runSession(ctx, conn)
	}
}

// PluginMain is the entrypoint called when the plugin is loaded.
func PluginMain(ctx context.Context, stack netstack.UserStack, params config.Params) { //nolint:deadcode
	port, err := params.GetPort("port")
	if err != nil {
		log.Errorf(err.Error())
		return
	}
	err = runServer(ctx, stack, port)
	if err != nil {
		log.Errorf(err.Error())
	}
}
