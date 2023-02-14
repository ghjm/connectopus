package ssh_jwt

import (
	"errors"
	"fmt"
	"github.com/chzyer/readline"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"io"
	"net"
	"os"
	"runtime"
	"strings"
)

type SSHAgent struct {
	agent.ExtendedAgent
	conn io.ReadWriteCloser
}

func getSSHAgentFromFile(keyFile string) (*SSHAgent, error) {
	keyPEM, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, fmt.Errorf("error reading key file: %w", err)
	}
	var key interface{}
	sshErr := &ssh.PassphraseMissingError{}
	if errors.As(err, &sshErr) {
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
	return &SSHAgent{
		ExtendedAgent: a.(agent.ExtendedAgent),
		conn:          nil,
	}, nil
}

// GetSSHAgent either connects to a running agent if keyFile is empty, or creates a new agent and loads the
// specified key into it.
func GetSSHAgent(keyFile string) (*SSHAgent, error) {
	if keyFile != "" {
		return getSSHAgentFromFile(keyFile)
	}
	var conn io.ReadWriteCloser
	var err error
	socket := os.Getenv("SSH_AUTH_SOCK")
	if socket != "" {
		conn, err = net.Dial("unix", socket)
		if err != nil {
			return nil, fmt.Errorf("error connecting to SSH_AUTH_SOCK: %w", err)
		}
	} else if runtime.GOOS == "windows" {
		conn, err = pageantNewConn()
		if err != nil {
			return nil, fmt.Errorf("error connecting to Pageant: %w", err)
		}
	}
	if conn == nil {
		return nil, fmt.Errorf("no SSH agent found")
	}
	return &SSHAgent{
		ExtendedAgent: agent.NewClient(conn),
		conn:          conn,
	}, nil
}

// Close closes the agent's underlying connection
func (a *SSHAgent) Close() error {
	if a == nil || a.conn == nil {
		return nil
	}
	return a.conn.Close()
}

// GetMatchingKeys retrieves public keys from an agent which contain keyText as a substring.
func GetMatchingKeys(a agent.Agent, keyText string) ([]*agent.Key, error) {
	keys, err := a.List()
	if err != nil {
		return nil, fmt.Errorf("error listing SSH keys: %w", err)
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("no SSH keys found")
	}
	var matchingKeys []*agent.Key
	for _, key := range keys {
		if keyText == "" || strings.Contains(key.String(), keyText) {
			matchingKeys = append(matchingKeys, key)
		}
	}
	return matchingKeys, nil
}
