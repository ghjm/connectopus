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
	"github.com/ghjm/connectopus/pkg/localui"
	"github.com/ghjm/connectopus/pkg/netopus"
	"github.com/ghjm/connectopus/pkg/proto"
	"github.com/ghjm/golib/pkg/exit_handler"
	"github.com/ghjm/golib/pkg/file_cleaner"
	"github.com/ghjm/golib/pkg/ssh_jwt"
	"github.com/golang-jwt/jwt/v4"
	"github.com/mattn/go-isatty"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/exp/slices"
	"io"
	"net"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Root Command
var socketFile string
var socketNode string
var rootCmd = &cobra.Command{
	Use:           "connectopus",
	Short:         "CLI for Connectopus",
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !(isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())) {
			err := uiCmd.RunE(cmd, args)
			if errors.Is(err, syscall.EADDRINUSE) {
				// Another instance of the UI server is running, so try opening a browser to it.
				// If this is ours, then we should have already set a cookie that will make this work.
				// This is mostly intended for the case where a user is double-clicking on a download.
				_ = localui.OpenWebBrowser("http://localhost:26663")
			}
			return err
		}
		return cmd.Usage()
	},
}

// Init Command
var identity string
var dataDir string
var keyFile string
var keyText string
var force bool
var initDomain string
var initSubnet string
var initIP string
var initBackend string
var configFilename string
var initRun bool
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new node",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		a, err := ssh_jwt.GetSSHAgent(keyFile)
		if err != nil {
			return fmt.Errorf("error initializing SSH agent: %w", err)
		}
		var keys []*agent.Key
		keys, err = ssh_jwt.GetMatchingKeys(a, keyText)
		_ = a.Close()
		if err != nil {
			return fmt.Errorf("error listing SSH keys: %w", err)
		}
		if len(keys) == 0 {
			return fmt.Errorf("no SSH keys found")
		}
		if len(keys) > 1 {
			return cpctl.ErrMultipleSSHKeys
		}
		key := proto.MarshalablePublicKey{
			PublicKey: keys[0],
			Comment:   keys[0].Comment,
		}
		fmt.Printf("Using SSH key %s [...] %s\n", key.PublicKey.Type(), key.Comment)

		var newCfg config.Config
		if configFilename == "" {
			if initDomain == "" {
				initDomain = "connectopus.test"
				fmt.Printf("Using default domain %s\n", initDomain)
			}
			if initSubnet == "" {
				initSubnet = proto.RandomSubnet(proto.ParseIP("fd00::"), 8, 64).String()
				fmt.Printf("Using new subnet %s\n", initSubnet)
			}
			_, parsedSubnet, err := proto.ParseCIDR(initSubnet)
			var parsedIP proto.IP
			if err != nil {
				return fmt.Errorf("error parsing subnet: %w", err)
			}
			if initIP == "" || strings.HasPrefix(initIP, "subnet::") {
				if initIP == "" {
					initIP = "subnet::1"
				}
				initIP = strings.TrimPrefix(initIP, "subnet::")
				subnetParts := strings.Split(parsedSubnet.String(), "::")
				if len(subnetParts) == 0 {
					return fmt.Errorf("cannot parse subnet")
				}
				parsedIP = proto.ParseIP(subnetParts[0] + "::" + initIP)
				fmt.Printf("Using generated IP address %s\n", parsedIP.String())
			} else {
				parsedIP = proto.ParseIP(initIP)
			}
			var backends map[string]config.Params
			if initBackend != "" {
				backends = make(map[string]config.Params)
				be := config.Params{}
				for _, p := range strings.Split(initBackend, ",") {
					kv := strings.SplitN(p, "=", 2)
					if len(kv) != 2 {
						return fmt.Errorf("invalid backend parameters")
					}
					be[kv[0]] = kv[1]
				}
				backends["b1"] = be
			}
			newCfg = config.Config{
				Global: config.Global{
					Domain:         initDomain,
					Subnet:         parsedSubnet,
					AuthorizedKeys: []proto.MarshalablePublicKey{key},
					LastUpdated:    time.Time{},
				},
				Nodes: map[string]config.Node{
					identity: {
						Address:  parsedIP,
						Backends: backends,
					},
				},
			}
		} else {
			data, err := os.ReadFile(configFilename)
			if err != nil {
				return fmt.Errorf("error reading config file: %w", err)
			}
			newCfg = config.Config{}
			err = newCfg.Unmarshal(data)
			if err != nil {
				return fmt.Errorf("error parsing config file: %w", err)
			}
			found := false
			for _, ak := range newCfg.Global.AuthorizedKeys {
				if ak.String() == key.String() {
					found = true
					break
				}
			}
			if !found {
				newCfg.Global.AuthorizedKeys = append(newCfg.Global.AuthorizedKeys, key)
			}
		}
		if dataDir == "" {
			ucd, err := os.UserConfigDir()
			if err != nil {
				return fmt.Errorf("error finding user config directory: %w", err)
			}
			dataDir = path.Join(ucd, "connectopus", identity)
		}
		_, err = os.Stat(dataDir)
		if err == nil {
			if force {
				err = os.RemoveAll(dataDir)
				if err != nil {
					return fmt.Errorf("error removing existing data dir: %w", err)
				}
			} else {
				return fmt.Errorf("data dir already exists; use --force to remove it")
			}
		}
		err = os.MkdirAll(dataDir, 0700)
		if err != nil {
			return fmt.Errorf("error creating data dir: %w", err)
		}
		var newCfgData []byte
		var sig []byte
		newCfgData, sig, err = connectopus.SignConfig(keyFile, keyText, newCfg, configFilename != "")
		if err != nil {
			return fmt.Errorf("error signing config data: %w", err)
		}
		err = os.WriteFile(path.Join(dataDir, "config.yml"), newCfgData, 0600)
		if err != nil {
			return fmt.Errorf("error writing config file: %w", err)
		}
		err = os.WriteFile(path.Join(dataDir, "config.sig"), sig, 0600)
		if err != nil {
			return fmt.Errorf("error writing config signature: %w", err)
		}
		if initRun {
			return nodeCmd.RunE(cmd, args)
		}
		return nil
	},
}

// Node Command
var logLevel string
var nodeCmd = &cobra.Command{
	Use:     "node",
	Short:   "Run a Connectopus node",
	Args:    cobra.NoArgs,
	Version: version.Version(),
	RunE: func(cmd *cobra.Command, args []string) error {
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
				return fmt.Errorf("invalid log level")
			}
		}

		ctx, cancel := context.WithCancel(context.Background())
		exit_handler.AddExitFunc(cancel)

		_, err := connectopus.RunNode(ctx, dataDir, identity)
		if err != nil {
			return err
		}

		log.Infof("node %s started", identity)
		<-ctx.Done()
		return nil
	},
}

// Get Token Command
var tokenDuration string
var getTokenCmd = &cobra.Command{
	Use:   "get-token",
	Short: "Generate an authentication token from an SSH key",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := cpctl.GenToken(keyFile, keyText, tokenDuration)
		if err != nil {
			return fmt.Errorf("error generating token: %w", err)
		}
		fmt.Printf("%s\n", s)
		return nil
	},
}

// Verify Token Command
var token string
var verbose bool
var verifyTokenCmd = &cobra.Command{
	Use:   "verify-token",
	Short: "Verify a previously generated authentication token",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		sm, err := ssh_jwt.SetupSigningMethod("connectopus", nil)
		if err != nil {
			return fmt.Errorf("error initializing JWT signing method: %w", err)
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
			return fmt.Errorf("error parsing JWT token: %w", err)
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
		return nil
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
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := cpctl.NewTokenAndSocketClient(socketFile, socketNode, keyFile, keyText, "", proxyTo)
		if err != nil {
			return err
		}
		var config *cpctl.GetConfig
		config, err = client.GetConfig(context.Background())
		if err != nil {
			return err
		}
		if configFilename == "" {
			fmt.Print(config.Config.Yaml)
		} else {
			err = os.WriteFile(configFilename, []byte(config.Config.Yaml), 0600)
			if err != nil {
				return err
			}
			fmt.Printf("Config saved to %s\n", configFilename)
		}
		return nil
	},
}

// Set Config Command
var setConfigCmd = &cobra.Command{
	Use:     "set",
	Aliases: []string{"write", "load", "upload"},
	Short:   "Update the running configuration from a file or stdin",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := cpctl.NewTokenAndSocketClient(socketFile, socketNode, keyFile, keyText, "", proxyTo)
		if err != nil {
			return err
		}
		var configFile *os.File
		if configFilename == "-" {
			configFile = os.Stdin
		} else {
			configFile, err = os.Open(configFilename)
			if err != nil {
				return err
			}
		}
		var yaml []byte
		yaml, err = io.ReadAll(configFile)
		if err != nil {
			return err
		}
		newCfg := config.Config{}
		err = newCfg.Unmarshal(yaml)
		if err != nil {
			return fmt.Errorf("error parsing yaml data: %w", err)
		}
		if len(newCfg.Global.AuthorizedKeys) == 0 {
			fmt.Printf("Warning: Authorized keys list is empty.  Re-using existing keys.\n")
			var oldGetConfig *cpctl.GetConfig
			oldGetConfig, err = client.GetConfig(context.Background())
			if err != nil {
				return fmt.Errorf("error retrieving old configuration: %w", err)
			}
			oldCfg := config.Config{}
			err = oldCfg.Unmarshal([]byte(oldGetConfig.Config.Yaml))
			if err != nil {
				return fmt.Errorf("error parsing old configuration: %w", err)
			}
			newCfg.Global.AuthorizedKeys = append(newCfg.Global.AuthorizedKeys, oldCfg.Global.AuthorizedKeys...)
		}
		var newCfgData []byte
		var sig []byte
		newCfgData, sig, err = connectopus.SignConfig(keyFile, keyText, newCfg, true)
		if err != nil {
			return fmt.Errorf("error signing config data: %w", err)
		}
		_, err = client.SetConfig(context.Background(), cpctl.ConfigUpdateInput{
			Yaml:      string(newCfgData),
			Signature: string(sig),
		})
		if err != nil {
			return err
		}
		return nil
	},
}

// Edit Config Command
var editConfigCmd = &cobra.Command{
	Use:   "edit",
	Short: "Edit the running configuration",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := cpctl.NewTokenAndSocketClient(socketFile, socketNode, keyFile, keyText, "", proxyTo)
		if err != nil {
			return fmt.Errorf("error opening socket client: %w", err)
		}
		var getCfg *cpctl.GetConfig
		getCfg, err = client.GetConfig(context.Background())
		if err != nil {
			return fmt.Errorf("error getting current config: %w", err)
		}
		var tmpFile *os.File
		tmpFile, err = os.CreateTemp("", "connectopus-config-*.yml")
		if err != nil {
			return fmt.Errorf("error creating temp file: %w", err)
		}
		file_cleaner.DeleteOnExit(tmpFile.Name())
		_, err = tmpFile.Write([]byte(getCfg.Config.Yaml))
		if err != nil {
			return fmt.Errorf("error writing temp file: %w", err)
		}
		err = tmpFile.Close()
		if err != nil {
			return fmt.Errorf("error closing temp file: %w", err)
		}
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = os.Getenv("VISUAL")
		}
		if editor == "" {
			return fmt.Errorf("unable to find an editor; please set EDITOR or VISUAL")
		}
		c := exec.Command(editor, tmpFile.Name())
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		err = c.Run()
		if err != nil {
			return fmt.Errorf("error running editor: %w", err)
		}
		var newYaml []byte
		newYaml, err = os.ReadFile(tmpFile.Name())
		if err != nil {
			return fmt.Errorf("error reading temp file: %w", err)
		}
		if string(newYaml) == getCfg.Config.Yaml {
			fmt.Printf("Config file unchanged.  Not updating.\n")
			return nil
		}
		cfg := config.Config{}
		err = cfg.Unmarshal(newYaml)
		if err != nil {
			return fmt.Errorf("error parsing new config yaml: %w", err)
		}
		var newCfgData []byte
		var sig []byte
		newCfgData, sig, err = connectopus.SignConfig(keyFile, keyText, cfg, true)
		if err != nil {
			return fmt.Errorf("error signing new config: %w", err)
		}
		_, err = client.SetConfig(context.Background(), cpctl.ConfigUpdateInput{
			Yaml:      string(newCfgData),
			Signature: string(sig),
		})
		if err != nil {
			return fmt.Errorf("error updating config: %w", err)
		}
		fmt.Printf("Configuration updated.\n")
		return nil
	},
}

// UI Command
var noBrowser bool
var localUIPort int
var localUIToken string
var uiCmd = &cobra.Command{
	Use:   "ui",
	Short: "Run the Connectopus UI",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := []localui.Opt{
			localui.WithToken(localUIToken),
		}
		if noBrowser {
			opts = append(opts, localui.WithBrowser(false))
		}
		if localUIPort != 0 {
			opts = append(opts, localui.WithLocalPort(uint16(localUIPort)))
		}
		if socketNode != "" {
			opts = append(opts, localui.WithNode(socketNode))
		}
		return localui.Serve(opts...)
	},
}

// Status Command
var proxyTo string
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Get status",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := cpctl.NewTokenAndSocketClient(socketFile, socketNode, keyFile, keyText, "", proxyTo)
		if err != nil {
			return err
		}
		var status *cpctl.GetStatus
		status, err = client.GetStatus(context.Background())
		if err != nil {
			return err
		}
		if status == nil {
			return fmt.Errorf("no status returned")
		}
		fmt.Printf("Node: %s [%s]\n", status.Status.Name, status.Status.Addr)
		fmt.Printf("  Global Settings:\n")
		fmt.Printf("    Domain: %s\n", status.Status.Global.Domain)
		fmt.Printf("    Subnet: %s\n", status.Status.Global.Subnet)
		fmt.Printf("    Config Last Updated: %s\n", status.Status.Global.ConfigLastUpdated)
		fmt.Printf("    Authorized Keys:\n")
		for _, keyStr := range status.Status.Global.AuthorizedKeys {
			if !verbose {
				keyStr, err = abbreviateKey(keyStr)
				if err != nil {
					fmt.Printf("      %s\n", fmt.Errorf("bad key: %w", err))
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
			slices.SortFunc(status.Status.Sessions, func(a, b *cpctl.GetStatus_Status_Sessions) int {
				if b == nil {
					return -1
				}
				if a == nil {
					return 1
				}
				if a.Addr < b.Addr {
					return 1
				}
				if a.Addr > b.Addr {
					return -1
				}
				return 0
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
			slices.SortFunc(status.Status.Nodes, func(a, b *cpctl.GetStatus_Status_Nodes) int {
				switch {
				case b == nil:
					return -1
				case a == nil:
					return 1
				case a.Name == "" && b.Name != "":
					return -1
				case a.Name != "" && b.Name == "":
					return 1
				}
				if a.Addr < b.Addr {
					return 1
				}
				if a.Addr > b.Addr {
					return -1
				}
				return 0
			})
			for _, node := range status.Status.Nodes {
				if node != nil {
					slices.SortFunc(node.Conns, func(a, b *cpctl.GetStatus_Status_Nodes_Conns) int {
						switch {
						case b == nil:
							return -1
						case a == nil:
							return 1
						}
						if a.Subnet < b.Subnet {
							return 1
						}
						if a.Subnet > b.Subnet {
							return -1
						}
						return 0
					})
					peerList := make([]string, 0)
					for _, p := range node.Conns {
						peerList = append(peerList, fmt.Sprintf("%s (%.2f)", formatNode(p.Subnet, nodeNames), p.Cost))
					}
					fmt.Printf("    %s --> %s\n", formatNode(node.Addr, nodeNames), strings.Join(peerList, ", "))
				}
			}
		}
		return nil
	},
}

// Nsenter Command
var namespaceName string
var nsenterCmd = &cobra.Command{
	Use:   "nsenter",
	Short: "Run a command within a network namespace",
	Args:  cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := cpctl.NewTokenAndSocketClient(socketFile, socketNode, keyFile, keyText, "", "")
		if err != nil {
			return err
		}
		nsName := &namespaceName
		if namespaceName == "" {
			nsName = nil
		}
		var list *cpctl.GetNetns
		list, err = client.GetNetns(context.Background(), nsName)
		if err != nil {
			return err
		}
		pid := 0
		if list != nil {
			for _, netns := range list.Netns {
				if netns != nil {
					if pid != 0 {
						return fmt.Errorf("multiple network namespaces found - please specify one")
					}
					pid = netns.Pid
				}
			}
		}
		if pid == 0 {
			return fmt.Errorf("no matching network namespaces found")
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
			return err
		}
		err = command.Wait()
		if err != nil {
			return err
		}
		return nil
	},
}

// Netns Shim (called internally - not used by users)
var netnsFd int
var netnsTunIf string
var netnsAddr string
var netnsMTU int
var netnsDomain string
var netnsDNSServer string
var netnsShimCmd = &cobra.Command{
	Use:     "netns_shim",
	Args:    cobra.NoArgs,
	Version: version.Version(),
	Hidden:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		err := netns.RunShim(netnsFd, netnsTunIf, uint16(netnsMTU), netnsAddr, netnsDomain, netnsDNSServer)
		if err != nil {
			return fmt.Errorf("error running netns shim: %w", err)
		}
		return nil
	},
}

// Setup-tunnel command
var uid int
var gid int
var check bool
var setupTunnelCmd = &cobra.Command{
	Use:   "setup-tunnel",
	Short: "Pre-create tunnel interface(s) for a node - usually run with sudo",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		config := &config.Config{}
		err := config.Load(configFilename)
		if err != nil {
			return fmt.Errorf("error reading config file: %w", err)
		}
		node, ok := config.Nodes[identity]
		if !ok {
			return fmt.Errorf("no such node in config file")
		}
		mtu := netopus.LeastMTU(node, 1500)
		if check {
			for name, tunDev := range node.TunDevs {
				_, err := tun.SetupLink(tunDev.DeviceName, net.IP(tunDev.Address), config.Global.Subnet.AsIPNet(),
					mtu, tun.WithCheck())
				if err != nil {
					return fmt.Errorf("check failed for %s: %w", name, err)
				}
			}
			return nil
		}
		if uid == -1 {
			uidStr := os.Getenv("SUDO_UID")
			if uidStr != "" {
				var err error
				uid, err = strconv.Atoi(uidStr)
				if err != nil {
					return fmt.Errorf("error parsing SUDO_UID: %w", err)
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
					return fmt.Errorf("error parsing SUDO_GID: %w", err)
				}
			} else {
				uid = os.Getgid()
			}
		}
		for name, tunDev := range node.TunDevs {
			_, err := tun.SetupLink(tunDev.DeviceName, net.IP(tunDev.Address), config.Global.Subnet.AsIPNet(),
				mtu, tun.WithUidGid(uint(uid), uint(gid)), tun.WithPersist())
			if err != nil {
				return fmt.Errorf("error setting up %s: %w", name, err)
			}
		}
		return nil
	},
}

// abbreviateKey returns a shorter, human-readable version of an SSH public key
func abbreviateKey(keyStr string) (string, error) {
	key, err := proto.ParseAuthorizedKey(keyStr)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s [...] %s", key.PublicKey.Type(), key.Comment), nil
}

func formatNode(addr string, nodeNames map[string]string) string {
	name := nodeNames[addr]
	if name == "" {
		return fmt.Sprintf("[%s]", addr)
	}
	return fmt.Sprintf("%s [%s]", name, addr)
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

	initCmd.Flags().StringVar(&identity, "id", "", "Node ID (required)")
	_ = initCmd.MarkFlagRequired("id")
	initCmd.Flags().StringVar(&dataDir, "datadir", "", "Data dir override")
	initCmd.Flags().StringVar(&keyText, "text", "", "Text to search for in SSH keys from agent")
	initCmd.Flags().StringVar(&keyFile, "key", "", "SSH private key file")
	initCmd.Flags().BoolVar(&force, "force", false, "Overwrite existing node config")
	initCmd.Flags().StringVar(&initDomain, "domain", "", "DNS domain name")
	initCmd.Flags().StringVar(&initSubnet, "subnet", "", "global subnet")
	initCmd.Flags().StringVar(&initIP, "ip", "", "node IP address")
	initCmd.Flags().StringVar(&initBackend, "backend", "", "initial backend for bootstrapping")
	initCmd.Flags().StringVar(&configFilename, "config", "", "config file to load")
	initCmd.Flags().BoolVar(&initRun, "run", false, "run node after initializing")
	initCmd.Flags().StringVar(&logLevel, "log-level", "", "Set log level (error/warning/info/debug)")

	nodeCmd.Flags().StringVar(&identity, "id", "", "Node ID (required)")
	_ = nodeCmd.MarkFlagRequired("id")
	nodeCmd.Flags().StringVar(&dataDir, "datadir", "", "Data dir override")
	nodeCmd.Flags().StringVar(&logLevel, "log-level", "", "Set log level (error/warning/info/debug)")

	netnsShimCmd.Flags().IntVar(&netnsFd, "fd", 0, "file descriptor")
	_ = netnsShimCmd.MarkFlagRequired("fd")
	netnsShimCmd.Flags().StringVar(&netnsTunIf, "tunif", "", "tun interface name")
	_ = netnsShimCmd.MarkFlagRequired("tunif")
	netnsShimCmd.Flags().StringVar(&netnsAddr, "addr", "", "ip address")
	_ = netnsShimCmd.MarkFlagRequired("addr")
	netnsShimCmd.Flags().IntVar(&netnsMTU, "mtu", 1500, "link MTU")
	netnsShimCmd.Flags().StringVar(&netnsDomain, "domain", "", "dns domain name")
	_ = netnsShimCmd.MarkFlagRequired("domain")
	netnsShimCmd.Flags().StringVar(&netnsDNSServer, "dns-server", "", "dns server")
	_ = netnsShimCmd.MarkFlagRequired("dns-server")

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
	uiCmd.Flags().BoolVar(&noBrowser, "stay-alive", false, "Do not close server after the last browser exits")
	uiCmd.Flags().StringVar(&socketFile, "socketfile", "",
		"Socket file to communicate between CLI and node")
	uiCmd.Flags().StringVar(&socketNode, "node", "",
		"Select which node's socket file to open, if there are multiple")
	uiCmd.Flags().IntVar(&localUIPort, "local-ui-port", 26663,
		"UI port on localhost")
	uiCmd.Flags().StringVar(&localUIToken, "local-ui-token", "", "")
	_ = uiCmd.Flags().MarkHidden("local-ui-token")

	setupTunnelCmd.Flags().StringVar(&configFilename, "config", "", "Config file name (required)")
	_ = setupTunnelCmd.MarkFlagRequired("config")
	setupTunnelCmd.Flags().StringVar(&identity, "id", "", "Node ID (required)")
	_ = setupTunnelCmd.MarkFlagRequired("id")
	setupTunnelCmd.Flags().IntVar(&uid, "uid", -1, "User ID who will own the tunnel")
	setupTunnelCmd.Flags().IntVar(&gid, "gid", -1, "Group ID who will own the tunnel")
	setupTunnelCmd.Flags().BoolVar(&check, "check", false, "Only check the tunnel - do not create")

	rootCmd.AddCommand(initCmd, nodeCmd, netnsShimCmd, statusCmd, getTokenCmd, verifyTokenCmd, uiCmd, configCmd)
	if runtime.GOOS == "linux" {
		rootCmd.AddCommand(nsenterCmd, setupTunnelCmd)
	}

	err := rootCmd.Execute()
	exitCode := 0
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		exitCode = 1
	}
	exit_handler.RunExitFuncs()
	os.Exit(exitCode)
}
