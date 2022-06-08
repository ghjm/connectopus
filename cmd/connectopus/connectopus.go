package main

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/internal/version"
	"github.com/ghjm/connectopus/pkg/backends/backend_registry"
	"github.com/ghjm/connectopus/pkg/config"
	"github.com/ghjm/connectopus/pkg/cpctl"
	"github.com/ghjm/connectopus/pkg/links"
	"github.com/ghjm/connectopus/pkg/links/netns"
	"github.com/ghjm/connectopus/pkg/links/tun"
	"github.com/ghjm/connectopus/pkg/netopus"
	"github.com/ghjm/connectopus/pkg/services"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"net"
	"os"
	"path"
)

func errHalt(err error) {
	_, _ = fmt.Fprintf(os.Stderr, "Error: %s\n", err)
	os.Exit(1)
}

var configFile string
var identity string
var logLevel string
var rootCmd = &cobra.Command{
	Use:     "connectopus",
	Args:    cobra.NoArgs,
	Version: version.Version(),
	Run: func(cmd *cobra.Command, args []string) {
		config, err := config.LoadConfig(configFile)
		if err != nil {
			errHalt(err)
		}
		if logLevel != "" {
			switch logLevel {
			case "error":
				log.SetLevel(log.ErrorLevel)
			case "warning":
				log.SetLevel(log.WarnLevel)
			case "info":
				log.SetLevel(log.InfoLevel)
			case "debug":
				log.SetLevel(log.DebugLevel)
			default:
				errHalt(fmt.Errorf("invalid log level"))
			}
		}
		node, ok := config.Nodes[identity]
		if !ok {
			errHalt(fmt.Errorf("node ID not found in config file"))
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		subnet := net.IPNet(config.Global.Subnet)
		addr := net.IP(node.Address)
		var n netopus.Netopus
		n, err = netopus.NewNetopus(ctx, &subnet, addr)
		if err != nil {
			errHalt(err)
		}
		if len(node.Tun.Address) > 0 {
			var tunLink links.Link
			tunLink, err = tun.New(ctx, node.Tun.Name, net.IP(node.Tun.Address), &subnet)
			if err != nil {
				errHalt(err)
			}
			tunCh := tunLink.SubscribePackets()
			go func() {
				for {
					select {
					case <-ctx.Done():
						tunLink.UnsubscribePackets(tunCh)
						return
					case packet := <-tunCh:
						_ = n.SendPacket(packet)
					}
				}
			}()
			n.AddExternalRoute(net.IP(node.Tun.Address), tunLink.SendPacket)
		}
		for _, backend := range node.Backends {
			err = backend_registry.RunBackend(ctx, n, backend.BackendType, backend.Params)
			if err != nil {
				errHalt(err)
			}
		}
		for _, service := range node.Services {
			err = services.RunService(ctx, n, service)
			if err != nil {
				errHalt(err)
			}
		}
		nsreg := &netns.Registry{}
		for _, namespace := range node.Namespaces {
			var ns *netns.Link
			ns, err = netns.New(ctx, net.IP(namespace.Address))
			if err != nil {
				errHalt(err)
			}
			nsCh := ns.SubscribePackets()
			go func() {
				for {
					select {
					case <-ctx.Done():
						ns.UnsubscribePackets(nsCh)
						return
					case packet := <-nsCh:
						_ = n.SendPacket(packet)
					}
				}
			}()
			n.AddExternalRoute(net.IP(namespace.Address), ns.SendPacket)
			nsreg.Add(namespace.Name, ns.PID())
		}
		if node.Cpctl.Enable {
			csrv := cpctl.Resolver{
				NsReg: nsreg,
			}
			socketFile := path.Join(os.Getenv("HOME"), ".local", "share", "connectopus", "cpctl.sock")
			if node.Cpctl.SocketFile != "" {
				socketFile = node.Cpctl.SocketFile
			}
			err = csrv.ServeUnix(ctx, socketFile)
			if err != nil {
				errHalt(err)
			}
			if node.Cpctl.Port != 0 {
				err = csrv.ServeHTTP(ctx, node.Cpctl.Port)
				if err != nil {
					errHalt(err)
				}
			}
		}
		<-ctx.Done()
	},
}

var netnsFd int
var netnsTunIf string
var netnsAddr string
var netnsCmd = &cobra.Command{
	Use:     "netns_shim",
	Args:    cobra.NoArgs,
	Version: version.Version(),
	Hidden:  true,
	Run: func(cmd *cobra.Command, args []string) {
		err := netns.RunShim(netnsFd, netnsTunIf, netnsAddr)
		if err != nil {
			errHalt(err)
		}
	},
}

func main() {
	rootCmd.Flags().StringVar(&configFile, "config", "", "Config file name (required)")
	_ = rootCmd.MarkFlagRequired("config")
	rootCmd.Flags().StringVar(&identity, "id", "", "Node ID (required)")
	_ = rootCmd.MarkFlagRequired("identity")
	rootCmd.Flags().StringVar(&logLevel, "log-level", "", "Set log level (error/warning/info/debug)")

	netnsCmd.Flags().IntVar(&netnsFd, "fd", 0, "file descriptor")
	_ = netnsCmd.MarkFlagRequired("fd")
	netnsCmd.Flags().StringVar(&netnsTunIf, "tunif", "", "tun interface name")
	_ = netnsCmd.MarkFlagRequired("fd")
	netnsCmd.Flags().StringVar(&netnsAddr, "addr", "", "ip address")
	_ = netnsCmd.MarkFlagRequired("addr")

	rootCmd.AddCommand(netnsCmd)

	err := rootCmd.Execute()
	if err != nil {
		errHalt(err)
	}
}
