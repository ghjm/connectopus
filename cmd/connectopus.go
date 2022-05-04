package main

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/internal/version"
	"github.com/ghjm/connectopus/pkg/backends/backend_registry"
	"github.com/ghjm/connectopus/pkg/config"
	"github.com/ghjm/connectopus/pkg/netopus"
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
			n.AddExternalRoute(net.IP(node.Tun.Address), func(packet []byte) error {
				fmt.Printf("tunnel got external packet")
				return nil
			})
		}
		for _, backend := range node.Backends {
			err = backend_registry.RunBackend(ctx, n, backend.BackendType, backend.Params)
			if err != nil {
				errHalt(err)
			}
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
