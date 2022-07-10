package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ghjm/connectopus/internal/version"
	"github.com/ghjm/connectopus/pkg/config"
	"github.com/ghjm/connectopus/pkg/connectopus"
	"github.com/ghjm/connectopus/pkg/cpctl"
	"github.com/ghjm/connectopus/pkg/links/netns"
	"github.com/ghjm/connectopus/pkg/links/tun"
	"github.com/ghjm/connectopus/pkg/netopus"
	"github.com/ghjm/connectopus/pkg/proto"
	"github.com/ghjm/connectopus/pkg/x/bridge"
	"github.com/ghjm/connectopus/pkg/x/exit_handler"
	"github.com/ghjm/connectopus/pkg/x/file_cleaner"
	"github.com/ghjm/connectopus/pkg/x/ssh_jwt"
	"github.com/golang-jwt/jwt/v4"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"golang.org/x/exp/slices"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Root Command
var socketFile string
var socketNode string
var rootCmd = &cobra.Command{
	Use:   "connectopus",
	Short: "CLI for Connectopus",
}

// Node Command
var configFilename string
var identity string
var logLevel string
var nodeCmd = &cobra.Command{
	Use:     "node",
	Short:   "Run a Connectopus node",
	Args:    cobra.NoArgs,
	Version: version.Version(),
	Run: func(cmd *cobra.Command, args []string) {
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
				errExitf("invalid log level")
			}
		}

		cfgData, err := ioutil.ReadFile(configFilename)
		if err != nil {
			errExitf("error loading config file: %s", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		exit_handler.AddExitFunc(cancel)

		_, err = connectopus.RunNode(ctx, cfgData, identity)
		if err != nil {
			errExit(err)
		}

		log.Infof("node %s started", identity)
		<-ctx.Done()
	},
}

// Get Token Command
var keyFile string
var keyText string
var tokenDuration string
var getTokenCmd = &cobra.Command{
	Use:   "get-token",
	Short: "Generate an authentication token from an SSH key",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		s, err := cpctl.GenToken(keyFile, keyText, tokenDuration)
		if err != nil {
			errExitf("error generating token: %s", err)
		}
		fmt.Printf("%s\n", s)
	},
}

// Verify Token Command
var token string
var verbose bool
var verifyTokenCmd = &cobra.Command{
	Use:   "verify-token",
	Short: "Verify a previously generated authentication token",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		sm, err := ssh_jwt.SetupSigningMethod("connectopus", nil)
		if err != nil {
			errExitf("error initializing JWT signing method: %s", err)
		}
		var parsedToken *jwt.Token
		parsedToken, err = jwt.Parse(token, func(t *jwt.Token) (interface{}, error) {
			pubkey, ok := t.Header["kid"]
			if !ok {
				return nil, fmt.Errorf("no pubkey")
			}
			return pubkey, nil
		}, jwt.WithValidMethods([]string{sm.Alg()}))
		if err != nil {
			errExitf("error parsing JWT token: %s", err)
		}
		var signingKey *string
		switch kid := parsedToken.Header["kid"].(type) {
		case string:
			signingKey = &kid
			if !verbose {
				abbrevKey, err := abbreviateKey(kid)
				if err == nil {
					signingKey = &abbrevKey
				}
			}
		}
		var expirationDate *time.Time
		claims, ok := parsedToken.Claims.(jwt.MapClaims)
		if ok {
			switch iat := claims["exp"].(type) {
			case float64:
				tm := time.Unix(int64(iat), 0)
				expirationDate = &tm
			case json.Number:
				v, _ := iat.Int64()
				tm := time.Unix(v, 0)
				expirationDate = &tm
			}
		}

		fmt.Printf("Token is valid.\n")
		if signingKey != nil {
			fmt.Printf("  Signed By:  %s\n", *signingKey)
		}
		if expirationDate != nil {
			fmt.Printf("  Expiration: %s\n", expirationDate.String())
		}
	},
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Commands involving configuration",
}

// Get Config Command
var getConfigCmd = &cobra.Command{
	Use:     "get",
	Aliases: []string{"read", "save", "download", "dump"},
	Short:   "Retrieve the running configuration and save it as YAML",
	Args:    cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		client, err := cpctl.NewTokenAndSocketClient(socketFile, socketNode, keyFile, keyText, "", proxyTo)
		if err != nil {
			errExit(err)
		}
		var config *cpctl.GetConfig
		config, err = client.GetConfig(context.Background())
		if err != nil {
			errExit(err)
		}
		if configFilename == "" {
			fmt.Printf(config.Config.Yaml)
		} else {
			err = ioutil.WriteFile(configFilename, []byte(config.Config.Yaml), 0600)
			if err != nil {
				errExit(err)
			}
			fmt.Printf("Config saved to %s\n", configFilename)
		}
	},
}

// Set Config Command
var setConfigCmd = &cobra.Command{
	Use:     "set",
	Aliases: []string{"write", "load", "upload"},
	Short:   "Update the running configuration from a file or stdin",
	Args:    cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		client, err := cpctl.NewTokenAndSocketClient(socketFile, socketNode, keyFile, keyText, "", proxyTo)
		if err != nil {
			errExit(err)
		}
		var configFile *os.File
		if configFilename == "-" {
			configFile = os.Stdin
		} else {
			configFile, err = os.Open(configFilename)
			if err != nil {
				errExit(err)
			}
		}
		var yaml []byte
		yaml, err = io.ReadAll(configFile)
		if err != nil {
			errExit(err)
		}
		_, err = client.SetConfig(context.Background(), cpctl.ConfigUpdateInput{
			Yaml:      string(yaml),
			Signature: "",
		})
		if err != nil {
			errExit(err)
		}
	},
}

// Edit Config Command
var editConfigCmd = &cobra.Command{
	Use:   "edit",
	Short: "Edit the running configuration",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		client, err := cpctl.NewTokenAndSocketClient(socketFile, socketNode, keyFile, keyText, "", proxyTo)
		if err != nil {
			errExitf("error opening socket client: %s", err)
		}
		var config *cpctl.GetConfig
		config, err = client.GetConfig(context.Background())
		if err != nil {
			errExitf("error getting current config: %s", err)
		}
		var tmpFile *os.File
		tmpFile, err = ioutil.TempFile("", "connectopus-config-*.yml")
		if err != nil {
			errExitf("error creating temp file: %s", err)
		}
		file_cleaner.DeleteOnExit(tmpFile.Name())
		_, err = tmpFile.Write([]byte(config.Config.Yaml))
		if err != nil {
			errExitf("error writing temp file: %s", err)
		}
		err = tmpFile.Close()
		if err != nil {
			errExitf("error closing temp file: %s", err)
		}
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = os.Getenv("VISUAL")
		}
		if editor == "" {
			errExitf("unable to find an editor.  Please set EDITOR or VISUAL.")
		}
		c := exec.Command(editor, tmpFile.Name())
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		err = c.Run()
		if err != nil {
			errExitf("error running editor: %s", err)
		}
		var newYaml []byte
		newYaml, err = ioutil.ReadFile(tmpFile.Name())
		if err != nil {
			errExitf("error reading temp file: %s", err)
		}
		if string(newYaml) == config.Config.Yaml {
			fmt.Printf("Config file unchanged.  Not updating.\n")
			return
		}
		_, err = client.SetConfig(context.Background(), cpctl.ConfigUpdateInput{
			Yaml:      string(newYaml),
			Signature: "",
		})
		if err != nil {
			errExitf("error updating config: %s", err)
		}
		fmt.Printf("Configuration updated.\n")
	},
}

// UI Command
var noBrowser bool
var localUIPort int
var uiCmd = &cobra.Command{
	Use:   "ui",
	Short: "Run the Connectopus UI",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if socketFile == "" {
			var err error
			socketFile, err = cpctl.FindSocketFile(socketNode)
			if err != nil {
				errExit(err)
			}
		}
		tokStr, err := cpctl.GenToken(keyFile, keyText, tokenDuration)
		if err != nil {
			errExitf("error generating token: %s", err)
		}
		var li net.Listener
		li, err = net.Listen("tcp", fmt.Sprintf("localhost:%d", localUIPort))
		if err != nil {
			errExitf("tcp listen error: %s", err)
		}
		url := fmt.Sprintf("http://localhost:%d/?AuthToken=%s", localUIPort, tokStr)
		if noBrowser {
			fmt.Printf("Server started on URL: %s\n", url)
		} else {
			fmt.Printf("Server started.  Launching browser.\n")
			err = openWebBrowser(url)
			if err != nil {
				errExitf("error opening browser: %s", err)
			}
		}
		for {
			c, err := li.Accept()
			if err != nil {
				errExitf("tcp accept error: %s", err)
			}
			go func() {
				uc, err := net.Dial("unix", socketFile)
				if err != nil {
					errExitf("unix socket connection error: %s", err)
				}
				err = bridge.RunBridge(c, uc)
				if err != nil {
					fmt.Printf("Bridge error: %s", err)
				}
			}()
		}
	},
}

// Status Command
var proxyTo string
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Get status",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		client, err := cpctl.NewTokenAndSocketClient(socketFile, socketNode, keyFile, keyText, "", proxyTo)
		if err != nil {
			errExit(err)
		}
		var status *cpctl.GetStatus
		status, err = client.GetStatus(context.Background())
		if err != nil {
			errExit(err)
		}
		if status == nil {
			errExit(fmt.Errorf("no status returned"))
		}
		fmt.Printf("Node: %s [%s]\n", status.Status.Name, status.Status.Addr)
		fmt.Printf("  Global Settings:\n")
		fmt.Printf("    Domain: %s\n", status.Status.Global.Domain)
		fmt.Printf("    Subnet: %s\n", status.Status.Global.Subnet)
		fmt.Printf("    Authorized Keys:\n")
		for _, keyStr := range status.Status.Global.AuthorizedKeys {
			if !verbose {
				keyStr, err = abbreviateKey(keyStr)
				if err != nil {
					fmt.Printf("      %s\n", fmt.Errorf("bad key: %s", err))
					continue
				}
			}
			fmt.Printf("      %s\n", keyStr)
		}
		nodeNames := make(map[string]string)
		if status.Status.Nodes != nil {
			for _, nn := range status.Status.Nodes {
				nodeNames[nn.Addr] = nn.Name
			}
		}
		if status.Status.Sessions != nil {
			fmt.Printf("  Sessions:\n")
			slices.SortFunc(status.Status.Sessions, func(a, b *cpctl.GetStatus_Status_Sessions) bool {
				if b == nil {
					return false
				}
				if a == nil {
					return true
				}
				return a.Addr < b.Addr
			})
			for _, sess := range status.Status.Sessions {
				if sess != nil {
					sessName := formatNode(sess.Addr, nodeNames)
					if sess.Connected {
						fmt.Printf("    %s (Connected since %s)\n", sessName, sess.ConnStart)
					} else {
						fmt.Printf("    %s (Disconnected)\n", sessName)
					}
				}
			}
		}
		if status.Status.Nodes != nil {
			fmt.Printf("  Network Map:\n")
			slices.SortFunc(status.Status.Nodes, func(a, b *cpctl.GetStatus_Status_Nodes) bool {
				switch {
				case b == nil:
					return false
				case a == nil:
					return true
				case a.Name == "" && b.Name != "":
					return false
				case a.Name != "" && b.Name == "":
					return true
				}
				return a.Addr < b.Addr
			})
			for _, node := range status.Status.Nodes {
				if node != nil {
					slices.SortFunc(node.Conns, func(a, b *cpctl.GetStatus_Status_Nodes_Conns) bool {
						switch {
						case b == nil:
							return false
						case a == nil:
							return true
						}
						return a.Subnet < b.Subnet
					})
					peerList := make([]string, 0)
					for _, p := range node.Conns {
						peerList = append(peerList, fmt.Sprintf("%s (%.2f)", formatNode(p.Subnet, nodeNames), p.Cost))
					}
					fmt.Printf("    %s --> %s\n", formatNode(node.Addr, nodeNames), strings.Join(peerList, ", "))
				}
			}
		}
	},
}

// Nsenter Command
var namespaceName string
var nsenterCmd = &cobra.Command{
	Use:   "nsenter",
	Short: "Run a command within a network namespace",
	Args:  cobra.ArbitraryArgs,
	Run: func(cmd *cobra.Command, args []string) {
		client, err := cpctl.NewTokenAndSocketClient(socketFile, socketNode, keyFile, keyText, "", "")
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

// Netns Shim (called internally - not used by users)
var netnsFd int
var netnsTunIf string
var netnsAddr string
var netnsMTU int
var netnsShimCmd = &cobra.Command{
	Use:     "netns_shim",
	Args:    cobra.NoArgs,
	Version: version.Version(),
	Hidden:  true,
	Run: func(cmd *cobra.Command, args []string) {
		err := netns.RunShim(netnsFd, netnsTunIf, uint16(netnsMTU), netnsAddr)
		if err != nil {
			errExitf("error running netns shim: %s", err)
		}
	},
}

// Setup-tunnel command
var uid int
var gid int
var setupTunnelCmd = &cobra.Command{
	Use:   "setup-tunnel",
	Short: "Pre-create tunnel interface(s) for a node - usually run with sudo",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if uid == -1 {
			uidStr := os.Getenv("SUDO_UID")
			if uidStr != "" {
				var err error
				uid, err = strconv.Atoi(uidStr)
				if err != nil {
					errExitf("error parsing SUDO_UID: %s", err)
				}
			} else {
				uid = os.Getuid()
			}
		}
		if gid == -1 {
			gidStr := os.Getenv("SUDO_GID")
			if gidStr != "" {
				var err error
				gid, err = strconv.Atoi(gidStr)
				if err != nil {
					errExitf("error parsing SUDO_GID: %s", err)
				}
			} else {
				uid = os.Getgid()
			}
		}
		config, err := config.LoadConfig(configFilename)
		if err != nil {
			errExitf("error reading config file: %s", err)
		}
		node, ok := config.Nodes[identity]
		if !ok {
			errExitf("no such node in config file")
		}
		mtu := netopus.LeastMTU(node, 1500)
		for name, tunDev := range node.TunDevs {
			_, err := tun.SetupLink(tunDev.DeviceName, net.IP(tunDev.Address), config.Global.Subnet.AsIPNet(),
				mtu, tun.WithUidGid(uint(uid), uint(gid)), tun.WithPersist())
			if err != nil {
				errExitf("error setting up %s: %s", name, err)
			}
		}
	},
}

// errExit halts the program on error
func errExit(err error) {
	if errors.Is(err, cpctl.ErrMultipleNode) {
		errExitf("%s\nSelect a unique node using --node or --socketfile, or set CPCTL_SOCK.", err)
	}
	fmt.Printf("Error: %s\n", err)
	exit_handler.RunExitFuncs()
	os.Exit(1)
}

// errExit halts the program on error with a formatted message
func errExitf(format string, args ...any) {
	fmt.Printf(fmt.Sprintf("Error: %s\n", format), args...)
	exit_handler.RunExitFuncs()
	os.Exit(1)
}

// abbreviateKey returns a shorter, human-readable version of an SSH public key
func abbreviateKey(keyStr string) (string, error) {
	key, err := proto.ParseAuthorizedKey(keyStr)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s [...] %s", key.PublicKey.Type(), key.Comment), nil
}

// openWebBrowser opens the default web browser to a given URL
func openWebBrowser(url string) error {
	var err error
	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		//nolint:goerr113
		err = fmt.Errorf("unsupported platform")
	}
	return err
}

func formatNode(addr string, nodeNames map[string]string) string {
	name := nodeNames[addr]
	if name == "" {
		return fmt.Sprintf("[%s]", addr)
	} else {
		return fmt.Sprintf("%s [%s]", name, addr)
	}
}

func main() {

	getConfigCmd.Flags().StringVar(&keyText, "text", "", "Text to search for in SSH keys from agent")
	getConfigCmd.Flags().StringVar(&keyFile, "key", "", "SSH private key file")
	getConfigCmd.Flags().StringVar(&socketFile, "socketfile", "",
		"Socket file to communicate between CLI and node")
	getConfigCmd.Flags().StringVar(&socketNode, "node", "",
		"Select which node's socket file to open, if there are multiple")
	getConfigCmd.Flags().StringVar(&proxyTo, "proxy-to", "",
		"Node to communicate with (non-local implies proxy)")
	getConfigCmd.Flags().StringVar(&configFilename, "filename", "",
		"Filename to save to (default is print to stdout)")

	setConfigCmd.Flags().StringVar(&keyText, "text", "", "Text to search for in SSH keys from agent")
	setConfigCmd.Flags().StringVar(&keyFile, "key", "", "SSH private key file")
	setConfigCmd.Flags().StringVar(&socketFile, "socketfile", "",
		"Socket file to communicate between CLI and node")
	setConfigCmd.Flags().StringVar(&socketNode, "node", "",
		"Select which node's socket file to open, if there are multiple")
	setConfigCmd.Flags().StringVar(&proxyTo, "proxy-to", "",
		"Node to communicate with (non-local implies proxy)")
	setConfigCmd.Flags().StringVar(&configFilename, "filename", "",
		"Filename to read from (- means read from stdin)")
	_ = setConfigCmd.MarkFlagRequired("filename")

	editConfigCmd.Flags().StringVar(&keyText, "text", "", "Text to search for in SSH keys from agent")
	editConfigCmd.Flags().StringVar(&keyFile, "key", "", "SSH private key file")
	editConfigCmd.Flags().StringVar(&socketFile, "socketfile", "",
		"Socket file to communicate between CLI and node")
	editConfigCmd.Flags().StringVar(&socketNode, "node", "",
		"Select which node's socket file to open, if there are multiple")
	editConfigCmd.Flags().StringVar(&proxyTo, "proxy-to", "",
		"Node to communicate with (non-local implies proxy)")

	configCmd.AddCommand(getConfigCmd, setConfigCmd, editConfigCmd)

	nodeCmd.Flags().StringVar(&configFilename, "config", "", "Config file name (required)")
	_ = nodeCmd.MarkFlagRequired("config")
	nodeCmd.Flags().StringVar(&identity, "id", "", "Node ID (required)")
	_ = nodeCmd.MarkFlagRequired("id")
	nodeCmd.Flags().StringVar(&logLevel, "log-level", "", "Set log level (error/warning/info/debug)")

	netnsShimCmd.Flags().IntVar(&netnsFd, "fd", 0, "file descriptor")
	_ = netnsShimCmd.MarkFlagRequired("fd")
	netnsShimCmd.Flags().StringVar(&netnsTunIf, "tunif", "", "tun interface name")
	_ = netnsShimCmd.MarkFlagRequired("fd")
	netnsShimCmd.Flags().StringVar(&netnsAddr, "addr", "", "ip address")
	_ = netnsShimCmd.MarkFlagRequired("addr")
	netnsShimCmd.Flags().IntVar(&netnsMTU, "mtu", 1500, "link MTU")

	statusCmd.Flags().StringVar(&keyText, "text", "", "Text to search for in SSH keys from agent")
	statusCmd.Flags().StringVar(&keyFile, "key", "", "SSH private key file")
	statusCmd.Flags().BoolVar(&verbose, "verbose", false, "Show extra detail")
	statusCmd.Flags().StringVar(&socketFile, "socketfile", "",
		"Socket file to communicate between CLI and node")
	statusCmd.Flags().StringVar(&socketNode, "node", "",
		"Select which node's socket file to open, if there are multiple")
	statusCmd.Flags().StringVar(&proxyTo, "proxy-to", "",
		"Node to communicate with (non-local implies proxy)")

	nsenterCmd.Flags().StringVar(&keyText, "text", "", "Text to search for in SSH keys from agent")
	nsenterCmd.Flags().StringVar(&keyFile, "key", "", "SSH private key file")
	nsenterCmd.Flags().StringVar(&namespaceName, "netns", "", "Name of network namespace")
	nsenterCmd.Flags().StringVar(&socketFile, "socketfile", "",
		"Socket file to communicate between CLI and node")
	nsenterCmd.Flags().StringVar(&socketNode, "node", "",
		"Select which node's socket file to open, if there are multiple")

	getTokenCmd.Flags().StringVar(&keyText, "text", "", "Text to search for in SSH keys from agent")
	getTokenCmd.Flags().StringVar(&keyFile, "key", "", "SSH private key file")
	getTokenCmd.Flags().StringVar(&tokenDuration, "duration", "", "Token duration (time duration or \"forever\")")

	verifyTokenCmd.Flags().StringVar(&token, "token", "", "Token to verify")
	_ = verifyTokenCmd.MarkFlagRequired("token")
	verifyTokenCmd.Flags().BoolVar(&verbose, "verbose", false, "Show extra detail")

	uiCmd.Flags().StringVar(&keyText, "text", "", "Text to search for in SSH keys from agent")
	uiCmd.Flags().StringVar(&keyFile, "key", "", "SSH private key file")
	uiCmd.Flags().StringVar(&tokenDuration, "duration", "", "Token duration (time duration or \"forever\")")
	uiCmd.Flags().BoolVar(&noBrowser, "no-browser", false, "Do not open a browser")
	uiCmd.Flags().StringVar(&socketFile, "socketfile", "",
		"Socket file to communicate between CLI and node")
	uiCmd.Flags().StringVar(&socketNode, "node", "",
		"Select which node's socket file to open, if there are multiple")
	uiCmd.Flags().IntVar(&localUIPort, "local-ui-port", 26663,
		"UI port on localhost")

	setupTunnelCmd.Flags().StringVar(&configFilename, "config", "", "Config file name (required)")
	_ = setupTunnelCmd.MarkFlagRequired("config")
	setupTunnelCmd.Flags().StringVar(&identity, "id", "", "Node ID (required)")
	_ = setupTunnelCmd.MarkFlagRequired("id")
	setupTunnelCmd.Flags().IntVar(&uid, "uid", -1, "User ID who will own the tunnel")
	setupTunnelCmd.Flags().IntVar(&gid, "gid", -1, "Group ID who will own the tunnel")

	rootCmd.AddCommand(nodeCmd, netnsShimCmd, statusCmd, getTokenCmd, verifyTokenCmd, uiCmd, configCmd)
	if runtime.GOOS == "linux" {
		rootCmd.AddCommand(nsenterCmd, setupTunnelCmd)
	}

	err := rootCmd.Execute()
	if err != nil {
		fmt.Println(err)
		exit_handler.RunExitFuncs()
		os.Exit(1)
	}
	exit_handler.RunExitFuncs()
}
