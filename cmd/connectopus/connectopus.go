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
	"github.com/ghjm/connectopus/pkg/proto"
	"github.com/ghjm/connectopus/pkg/services"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"net"
	"os"
)

func errHalt(err error) {
	_, _ = fmt.Fprintf(os.Stderr, "Error: %s\n", err)
	os.Exit(1)
}

func defaultCost(cost float32) float32 {
	if cost <= 0 {
		return 1.0
	}
	return cost
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
		var n proto.Netopus
		n, err = netopus.New(ctx, node.Address, identity, node.MTU)
		if err != nil {
			errHalt(err)
		}
		for _, tunDev := range node.TunDevs {
			var tunLink links.Link
			tunLink, err = tun.New(ctx, tunDev.DeviceName, net.IP(tunDev.Address), config.Global.Subnet.AsIPNet())
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
			n.AddExternalRoute(tunDev.Name, proto.NewHostOnlySubnet(tunDev.Address), defaultCost(tunDev.Cost), tunLink.SendPacket)
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
			n.AddExternalRoute(namespace.Name, proto.NewHostOnlySubnet(namespace.Address), defaultCost(namespace.Cost), ns.SendPacket)
			nsreg.Add(namespace.Name, ns.PID())
		}
		csrv := cpctl.Resolver{
			C:     config,
			N:     n,
			NsReg: nsreg,
		}
		{
			li, err := n.ListenTCP(277)
			if err != nil {
				errHalt(err)
			}
			err = csrv.ServeAPI(ctx, li)
			if err != nil {
				errHalt(err)
			}
		}
		if node.Cpctl.SocketFile != "" {
			socketFile := os.ExpandEnv(node.Cpctl.SocketFile)
			err = csrv.ServeUnix(ctx, socketFile)
			if err != nil {
				errHalt(err)
			}
		}
		if node.Cpctl.Port != 0 {
			li, err := net.Listen("tcp", fmt.Sprintf(":%d", node.Cpctl.Port))
			if err != nil {
				errHalt(err)
			}
			err = csrv.ServeHTTP(ctx, li)
			if err != nil {
				errHalt(err)
			}
		}
		for _, backend := range node.Backends {
			err = backend_registry.RunBackend(ctx, n, backend.BackendType, defaultCost(backend.Cost), backend.Params)
			if err != nil {
				errHalt(err)
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
