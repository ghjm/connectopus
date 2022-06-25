package ssh_jwt

import (
	"encoding/base64"
	"fmt"
	"github.com/42wim/sshsig"
	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/crypto/ssh/agent"
	"strings"
)

type signingMethodSSHAgent struct {
	namespace   string
	agentClient agent.Agent
}

func NewSigningMethodSSHAgent(namespace string, a agent.Agent) (jwt.SigningMethod, error) {
	if a == nil {
		return nil, fmt.Errorf("no SSH agent")
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
