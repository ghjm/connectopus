package ssh_jwt

import (
	"encoding/json"
	"fmt"
	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/crypto/ssh/agent"
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
	keyStr, ok := key.(string)
	if !ok {
		return "", fmt.Errorf("key must be string")
	}
	return SignSSH(signingString, s.namespace, keyStr, s.agentClient)
}

func (s *signingMethodSSHAgent) Verify(signingString, signature string, key interface{}) error {
	keyStr, ok := key.(string)
	if !ok {
		return fmt.Errorf("key must be string")
	}
	return VerifySSHSignature(signingString, signature, s.namespace, keyStr)
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
