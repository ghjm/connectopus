package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/chzyer/readline"
	"github.com/ghjm/connectopus/pkg/cpctl"
	"github.com/ghjm/connectopus/pkg/proto"
	"github.com/ghjm/connectopus/pkg/x/ssh_jwt"
	"github.com/golang-jwt/jwt/v4"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/exp/slices"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"
)

func errExit(err error) {
	fmt.Printf("Error: %s\n", err)
	os.Exit(1)
}

func errExitf(format string, args ...any) {
	errStr := fmt.Sprintf(format, args...)
	fmt.Printf("Error: %s\n", errStr)
	os.Exit(1)
}

var socketFile string
var proxyNode string

var rootCmd = &cobra.Command{
	Use:   "cpctl",
	Short: "CLI for Connectopus",
}

func getSSHAgent(keyFile string) (agent.Agent, error) {
	if keyFile == "" {
		socket := os.Getenv("SSH_AUTH_SOCK")
		if socket == "" {
			return nil, fmt.Errorf("no SSH agent found")
		}
		conn, err := net.Dial("unix", socket)
		if err != nil {
			return nil, fmt.Errorf("failed to open SSH_AUTH_SOCK: %v", err)
		}
		return agent.NewClient(conn), nil
	}
	keyPEM, err := ioutil.ReadFile(keyFile)
	if err != nil {
		return nil, fmt.Errorf("error reading key file: %w", err)
	}
	var key interface{}
	key, err = ssh.ParseRawPrivateKey(keyPEM)
	if _, ok := err.(*ssh.PassphraseMissingError); ok {
		var rl *readline.Instance
		rl, err = readline.New("")
		if err != nil {
			return nil, fmt.Errorf("error initializing readline: %w", err)
		}
		var keyPassphrase []byte
		keyPassphrase, err = rl.ReadPassword("Enter SSH Key Passphrase: ")
		if err != nil {
			return nil, fmt.Errorf("error reading passphrase: %w", err)
		}
		key, err = ssh.ParseRawPrivateKeyWithPassphrase(keyPEM, keyPassphrase)
	}
	if err != nil {
		return nil, fmt.Errorf("error parsing SSH key: %w", err)
	}
	a := agent.NewKeyring()
	err = a.Add(agent.AddedKey{
		PrivateKey: key,
	})
	if err != nil {
		return nil, fmt.Errorf("error loading SSH key: %w", err)
	}
	return a, nil
}

func getUsableKey(a agent.Agent) (string, error) {
	keys, err := a.List()
	if err != nil {
		return "", fmt.Errorf("error listing SSH keys: %s", err)
	}
	if len(keys) == 0 {
		return "", fmt.Errorf("no SSH keys found")
	}
	var usableKeys []*agent.Key
	for _, key := range keys {
		if keyText == "" || strings.Contains(key.String(), keyText) {
			usableKeys = append(usableKeys, key)
		}
	}
	if len(usableKeys) == 0 {
		return "", fmt.Errorf("no usable SSH keys found")
	}
	if len(usableKeys) > 1 {
		return "", fmt.Errorf("multiple keys loaded in agent.  Use --text to select one uniquely.")
	}
	return usableKeys[0].String(), nil
}

func abbreviateKey(keyStr string) (string, error) {
	key, err := proto.ParseAuthorizedKey(keyStr)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s [...] %s", key.PublicKey.Type(), key.Comment), nil
}

var keyText string
var keyFile string
var tokenDuration string
var verbose bool
var getTokenCmd = &cobra.Command{
	Use:   "get-token",
	Short: "Generate an authentication token from an SSH key",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		a, err := getSSHAgent(keyFile)
		if err != nil {
			errExitf("error initializing SSH agent: %s", err)
		}
		key, err := getUsableKey(a)
		if err != nil {
			errExit(err)
		}

		var sm jwt.SigningMethod
		sm, err = ssh_jwt.NewSigningMethodSSHAgent("connectopus-cpctl", a)
		if err != nil {
			errExitf("error initializing JWT signing method: %s", err)
		}
		jwt.RegisterSigningMethod(sm.Alg(), func() jwt.SigningMethod { return sm })
		claims := &jwt.RegisteredClaims{
			Subject: "connectopus-cpctl auth",
		}
		if tokenDuration != "forever" {
			var expireDuration time.Duration
			if tokenDuration == "" {
				expireDuration = 24 * time.Hour
			} else {
				expireDuration, err = time.ParseDuration(tokenDuration)
				if err != nil {
					errExitf("error parsing token duration: %s", err)
				}
			}
			claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(expireDuration))
		}
		tok := jwt.NewWithClaims(sm, claims)
		tok.Header["kid"] = key
		var s string
		s, err = tok.SignedString(key)
		if err != nil {
			errExitf("error signing token: %s", err)
		}
		fmt.Printf("%s\n", s)
	},
}

var token string
var verifyTokenCmd = &cobra.Command{
	Use:   "verify-token",
	Short: "Verify a previously generated authentication token",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		a, err := getSSHAgent(keyFile)
		if err != nil {
			errExitf("error initializing SSH agent: %s", err)
		}
		var sm jwt.SigningMethod
		sm, err = ssh_jwt.NewSigningMethodSSHAgent("connectopus-cpctl", a)
		if err != nil {
			errExitf("error initializing JWT signing method: %s", err)
		}
		jwt.RegisterSigningMethod(sm.Alg(), func() jwt.SigningMethod { return sm })
		var parsedToken *jwt.Token
		parsedToken, err = jwt.Parse(token, func(t *jwt.Token) (interface{}, error) {
			pubkey, ok := t.Header["kid"]
			if !ok {
				return nil, fmt.Errorf("no pubkey")
			}
			return pubkey, nil
		}, jwt.WithValidMethods([]string{sm.Alg()}))
		if err != nil {
			errExit(err)
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

func formatNode(addr string, nodeNames map[string]string) string {
	name := nodeNames[addr]
	if name == "" {
		return fmt.Sprintf("[%s]", addr)
	} else {
		return fmt.Sprintf("%s [%s]", name, addr)
	}
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Get status",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		client, err := cpctl.NewSocketClient(socketFile, proxyNode)
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

var namespaceName string
var nsenterCmd = &cobra.Command{
	Use:   "nsenter",
	Short: "Run a command within a network namespace",
	Args:  cobra.ArbitraryArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if proxyNode != "" {
			errExit(fmt.Errorf("cannot enter a non-local namespace"))
		}
		client, err := cpctl.NewSocketClient(socketFile, proxyNode)
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
		home, err := os.UserHomeDir()
		if err != nil {
			errExitf("could not get user home dir: %s", err)
		}
		defaultSocketFile = path.Join(home, ".local", "share", "connectopus", "cpctl.sock")
	}
	rootCmd.PersistentFlags().StringVar(&socketFile, "socket-file", defaultSocketFile,
		"Socket file to communicate with Connectopus")
	rootCmd.PersistentFlags().StringVar(&proxyNode, "proxy-to", "",
		"Node to communicate with (non-local implies proxy)")

	statusCmd.Flags().BoolVar(&verbose, "verbose", false, "Show extra detail")

	nsenterCmd.Flags().StringVar(&namespaceName, "netns", "", "Name of network namespace")

	getTokenCmd.Flags().StringVar(&keyText, "text", "", "Text to search for in SSH keys from agent")
	getTokenCmd.Flags().StringVar(&keyFile, "key", "", "SSH private key file")
	getTokenCmd.Flags().StringVar(&tokenDuration, "duration", "", "Token duration (time duration or \"forever\")")

	verifyTokenCmd.Flags().StringVar(&token, "token", "", "Token to verify")
	_ = verifyTokenCmd.MarkFlagRequired("token")
	verifyTokenCmd.Flags().BoolVar(&verbose, "verbose", false, "Show extra detail")

	rootCmd.AddCommand(statusCmd, nsenterCmd, getTokenCmd, verifyTokenCmd)

	err := rootCmd.Execute()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
