package main

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/pkg/cpctl"
	"github.com/spf13/cobra"
	"os"
	"os/exec"
	"path"
	"strconv"
)

func errExit(err error) {
	fmt.Printf("Error: %s\n", err)
	os.Exit(1)
}

var socketFile string
var rootCmd = &cobra.Command{
	Use:   "cpctl",
	Short: "CLI for Connectopus",
}

var namespaceName string
var nsenterCmd = &cobra.Command{
	Use:   "nsenter",
	Short: "Run a command within a network namespace",
	Args:  cobra.ArbitraryArgs,
	Run: func(cmd *cobra.Command, args []string) {
		client, err := cpctl.NewSocketClient(socketFile)
		if err != nil {
			errExit(err)
		}
		nsName := &namespaceName
		if namespaceName == "" {
			nsName = nil
		}
		var list *cpctl.GetNetns
		list, err = client.GetNetns(context.Background(), nsName)
		if err != nil {
			errExit(err)
		}
		pid := 0
		if list != nil {
			for _, netns := range list.Netns {
				if netns != nil {
					if pid != 0 {
						errExit(fmt.Errorf("multiple network namespaces found - please specify one"))
					}
					pid = netns.Pid
				}
			}
		}
		if pid == 0 {
			errExit(fmt.Errorf("no matching network namespaces found"))
		}
		if len(args) == 0 {
			shell := os.Getenv("SHELL")
			if shell == "" {
				shell = "/bin/sh"
			}
			args = []string{shell}
		}
		args = append([]string{"--preserve-credentials", "--user", "--mount", "--net",
			"--uts", "-t", strconv.Itoa(pid)}, args...)
		command := exec.Command("nsenter", args...)
		command.Stdin = os.Stdin
		command.Stdout = os.Stdout
		command.Stderr = os.Stderr
		err = command.Start()
		if err != nil {
			errExit(err)
		}
		err = command.Wait()
		if err != nil {
			errExit(err)
		}
	},
}

func main() {
	defaultSocketFile := os.Getenv("CPCTL_SOCKET")
	if defaultSocketFile == "" {
		defaultSocketFile = path.Join(os.Getenv("HOME"), ".local", "share", "connectopus", "cpctl.sock")
	}
	rootCmd.PersistentFlags().StringVar(&socketFile, "socket-file", defaultSocketFile,
		"Socket file to communicate with Connectopus")

	nsenterCmd.Flags().StringVar(&namespaceName, "netns", "", "Name of network namespace")
	rootCmd.AddCommand(nsenterCmd)

	err := rootCmd.Execute()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
