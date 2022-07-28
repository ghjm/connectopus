package ssh_jwt

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/crypto/ssh/agent"
	"testing"
	"time"
)

func TestSSHJWT(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	a := agent.NewKeyring()
	err = a.Add(agent.AddedKey{
		PrivateKey: key,
	})
	if err != nil {
		t.Fatal(err)
	}
	var keys []*agent.Key
	keys, err = a.List()
	if len(keys) != 1 {
		t.Fatal("key failed to load or mysterious key appeared")
	}
	pubkeyStr := keys[0].String()

	var sm jwt.SigningMethod
	sm, err = SetupSigningMethod("testing", a)
	if err != nil {
		t.Fatal(err)
	}

	claims := &jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(2 * time.Second)),
		Subject:   "this is a test",
	}
	tok := jwt.NewWithClaims(sm, claims)
	tok.Header["kid"] = pubkeyStr
	var s string
	s, err = tok.SignedString(pubkeyStr)
	if err != nil {
		t.Fatal(err)
	}

	var token *jwt.Token
	checkToken := func() {
		token, err = jwt.Parse(s, func(t *jwt.Token) (interface{}, error) {
			pubkey, ok := t.Header["kid"]
			if !ok {
				return nil, fmt.Errorf("no pubkey")
			}
			return pubkey, nil
		}, jwt.WithValidMethods([]string{sm.Alg()}))
	}

	checkToken()
	if err != nil || !token.Valid {
		t.Fatal(err)
	}

	time.Sleep(3 * time.Second)

	checkToken()
	if err == nil && token.Valid {
		t.Fatal("expired token succeeded")
	}

	err = a.RemoveAll()
	if err != nil {
		t.Fatal(err)
	}
	checkToken()
	if err == nil && token.Valid {
		t.Fatal("token succeeded without key")
	}
}
