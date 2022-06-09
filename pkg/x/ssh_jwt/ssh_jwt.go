package ssh_jwt

import (
	"fmt"
	"github.com/42wim/sshsig"
	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/crypto/ssh/agent"
	"net"
	"os"
	"strings"
)

type signingMethodSSHAgent struct {
	namespace   string
	agentClient agent.Agent
}

func NewSigningMethodSSHAgent(namespace string, a agent.Agent) (jwt.SigningMethod, error) {
	if a == nil {
		socket := os.Getenv("SSH_AUTH_SOCK")
		if socket == "" {
			return nil, fmt.Errorf("no SSH agent found")
		}
		conn, err := net.Dial("unix", socket)
		if err != nil {
			return nil, fmt.Errorf("failed to open SSH_AUTH_SOCK: %v", err)
		}
		a = agent.NewClient(conn)
	}
	s := &signingMethodSSHAgent{
		namespace:   namespace,
		agentClient: a,
	}
	return s, nil
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
	return string(sig), nil
}

func (s *signingMethodSSHAgent) Verify(signingString, signature string, key interface{}) error {
	keyStr, ok := key.(string)
	if !ok {
		return fmt.Errorf("key must be string")
	}
	return sshsig.Verify(strings.NewReader(signingString), []byte(signature), []byte(keyStr), s.namespace)
}

func (s *signingMethodSSHAgent) Alg() string {
	return fmt.Sprintf("SSH-signature-%s", s.namespace)
}
