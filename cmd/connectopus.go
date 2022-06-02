package main

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/internal/version"
	"github.com/ghjm/connectopus/pkg/backends/backend_registry"
	"github.com/ghjm/connectopus/pkg/config"
	"github.com/ghjm/connectopus/pkg/netopus"
	"github.com/ghjm/connectopus/pkg/netopus/link"
	"github.com/ghjm/connectopus/pkg/netopus/link/netns"
	"github.com/ghjm/connectopus/pkg/netopus/link/tun"
	"github.com/ghjm/connectopus/plugins"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"net"
	"os"
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
			var tunLink link.Link
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
		for _, plugin := range node.Plugins {
			err = plugins.RunPlugin(ctx, plugin.File, n, plugin.Params)
			if err != nil {
				errHalt(err)
			}
		}
		for _, namespace := range node.Namespaces {
			var ns link.Link
			ns, err = netns.NewNetns(ctx, net.IP(namespace.Address))
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
		}
		<-ctx.Done()
	},
}

func main() {
	rootCmd.Flags().StringVar(&configFile, "config", "", "Config file name (required)")
	_ = rootCmd.MarkFlagRequired("config")
	rootCmd.Flags().StringVar(&identity, "id", "", "Node ID (required)")
	_ = rootCmd.MarkFlagRequired("identity")
	rootCmd.Flags().StringVar(&logLevel, "log-level", "", "Set log level (error/warning/info/debug)")
	err := rootCmd.Execute()
	if err != nil {
		errHalt(err)
	}
}
