package ssh_jwt

import (
	"encoding/base64"
	"fmt"
	"github.com/42wim/sshsig"
	"github.com/chzyer/readline"
	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"io/ioutil"
	"net"
	"os"
	"strings"
)

type signingMethodSSHAgent struct {
	namespace   string
	agentClient agent.Agent
}

// GetSSHAgent either connects to a running agent if keyFile is empty, or creates a new agent and loads the
// specified key into it.
func GetSSHAgent(keyFile string) (agent.Agent, error) {
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

// GetMatchingKeys retrieves public keys from an agent which contain keyText as a substring.
func GetMatchingKeys(a agent.Agent, keyText string) ([]string, error) {
	keys, err := a.List()
	if err != nil {
		return nil, fmt.Errorf("error listing SSH keys: %s", err)
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("no SSH keys found")
	}
	var matchingKeys []string
	for _, key := range keys {
		if keyText == "" || strings.Contains(key.String(), keyText) {
			matchingKeys = append(matchingKeys, key.String())
		}
	}
	return matchingKeys, nil
}

// SetupSigningMethod creates and registers a JWT signing method using SSH keys
func SetupSigningMethod(namespace string, a agent.Agent) (jwt.SigningMethod, error) {
	if a == nil {
		return nil, fmt.Errorf("no SSH agent")
	}
	sm := &signingMethodSSHAgent{
		namespace:   namespace,
		agentClient: a,
	}
	jwt.RegisterSigningMethod(sm.Alg(), func() jwt.SigningMethod { return sm })
	return sm, nil
}

func (s *signingMethodSSHAgent) Sign(signingString string, key interface{}) (string, error) {
	keyStr, ok := key.(string)
	if !ok {
		return "", fmt.Errorf("key must be string")
	}
	sig, err := sshsig.SignWithAgent([]byte(keyStr), s.agentClient, strings.NewReader(signingString), s.namespace)
	if err != nil {
		return "", err
	}
	sigStr := strings.ReplaceAll(string(sig), "\n", "")
	sigStr = strings.TrimPrefix(sigStr, "-----BEGIN SSH SIGNATURE-----")
	sigStr = strings.TrimSuffix(sigStr, "-----END SSH SIGNATURE-----")
	sigDec, err := base64.StdEncoding.DecodeString(sigStr)
	if err != nil {
		return "", err
	}
	sigEnc := base64.RawURLEncoding.EncodeToString(sigDec)
	return sigEnc, nil
}

func (s *signingMethodSSHAgent) Verify(signingString, signature string, key interface{}) error {
	keyStr, ok := key.(string)
	if !ok {
		return fmt.Errorf("key must be string")
	}
	sigBytes, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil {
		return err
	}
	sigStr := "-----BEGIN SSH SIGNATURE-----\n" + base64.StdEncoding.EncodeToString(sigBytes) + "\n-----END SSH SIGNATURE-----\n"
	return sshsig.Verify(strings.NewReader(signingString), []byte(sigStr), []byte(keyStr), s.namespace)
}

func (s *signingMethodSSHAgent) Alg() string {
	return fmt.Sprintf("SSH-signature-%s", s.namespace)
}
