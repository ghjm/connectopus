package ssh_jwt

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/42wim/sshsig"
	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/crypto/ssh/agent"
	"strings"
	"time"
)

type signingMethodSSHAgent struct {
	namespace   string
	agentClient agent.Agent
}

// SetupSigningMethod creates and registers a JWT signing method using SSH keys
func SetupSigningMethod(namespace string, a agent.Agent) (jwt.SigningMethod, error) {
	sm := &signingMethodSSHAgent{
		namespace:   namespace,
		agentClient: a,
	}
	jwt.RegisterSigningMethod(sm.Alg(), func() jwt.SigningMethod { return sm })
	return sm, nil
}

func (s *signingMethodSSHAgent) Sign(signingString string, key interface{}) (string, error) {
	if s.agentClient == nil {
		return "", fmt.Errorf("no SSH agent")
	}
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

// AuthorizeToken verifies a given token and, if successful, returns the token's public key and expiration time.  A
// non-expiring token will have a nil expiration time.
func (s *signingMethodSSHAgent) AuthorizeToken(token string) (string, *time.Time, error) {
	parsedToken, err := jwt.Parse(token, func(t *jwt.Token) (interface{}, error) {
		pubkey, ok := t.Header["kid"]
		if !ok {
			return nil, fmt.Errorf("no pubkey")
		}
		return pubkey, nil
	}, jwt.WithValidMethods([]string{s.Alg()}))
	if err != nil {
		return "", nil, err
	}
	signingKey, ok := parsedToken.Header["kid"].(string)
	if !ok {
		return "", nil, fmt.Errorf("signing key is not a string")
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
	return signingKey, expirationDate, nil
}
