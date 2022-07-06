package cpctl

import (
	"context"
	"errors"
	"fmt"
	"github.com/ghjm/connectopus/pkg/x/ssh_jwt"
	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/crypto/ssh/agent"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"time"
)

func getUsableKey(a agent.Agent, keyText string) (string, error) {
	keys, err := ssh_jwt.GetMatchingKeys(a, keyText)
	if err != nil {
		return "", fmt.Errorf("error listing SSH keys: %s", err)
	}
	if len(keys) == 0 {
		return "", fmt.Errorf("no SSH keys found")
	}
	if len(keys) > 1 {
		return "", fmt.Errorf("multiple SSH keys found.  Use --text to select one uniquely.")
	}
	return keys[0], nil
}

// GenTokenFromAgent generates a token from an already-open agent using a key matching keyText
func GenTokenFromAgent(a agent.Agent, keyText string, tokenDuration string) (string, error) {
	key, err := getUsableKey(a, keyText)
	if err != nil {
		return "", fmt.Errorf("error getting SSH key: %s", err)
	}
	var sm jwt.SigningMethod
	sm, err = ssh_jwt.SetupSigningMethod("connectopus", a)
	if err != nil {
		return "", fmt.Errorf("error initializing JWT signing method: %s", err)
	}
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
				return "", fmt.Errorf("error parsing token duration: %s", err)
			}
		}
		claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(expireDuration))
	}
	tok := jwt.NewWithClaims(sm, claims)
	tok.Header["kid"] = key
	var tokstr string
	tokstr, err = tok.SignedString(key)
	if err != nil {
		return "", fmt.Errorf("error signing token: %s", err)
	}
	return tokstr, nil
}

// GenToken generates a token by either communicating with an agent, or loading the key from keyFile.
func GenToken(keyFile string, keyText string, tokenDuration string) (string, error) {
	a, err := ssh_jwt.GetSSHAgent(keyFile)
	if err != nil {
		return "", fmt.Errorf("error initializing SSH agent: %s", err)
	}
	defer func() {
		_ = a.Close()
	}()
	return GenTokenFromAgent(a, keyText, tokenDuration)
}

// NewSocketClient creates a new socket client using a given auth token
func NewSocketClient(socketFile string, authToken string, proxyTo string) (*Client, error) {
	clientURL := "http://cpctl.sock/query"
	if proxyTo != "" {
		clientURL = fmt.Sprintf("http://cpctl.sock/proxy/%s/query", proxyTo)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	_, err := os.Stat(socketFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		d := net.Dialer{}
		return d.DialContext(ctx, "unix", socketFile)
	}
	var jar *cookiejar.Jar
	jar, err = cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("error creating cookie jar: %w", err)
	}
	var parsedURL *url.URL
	parsedURL, err = url.Parse(clientURL)
	if err != nil {
		return nil, fmt.Errorf("error parsing client URL: %w", err)
	}
	jar.SetCookies(parsedURL, []*http.Cookie{
		{
			Name:     "AuthToken",
			Value:    authToken,
			Secure:   false,
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
		},
	})
	client := &http.Client{
		Transport: transport,
		Jar:       jar,
		Timeout:   10 * time.Second,
	}
	return NewClient(client, clientURL), nil
}

// NewTokenAndSocketClient generates a token and then uses it to create a new socket client
func NewTokenAndSocketClient(socketFile string, keyFile string, keyText string, tokenDuration string, proxyTo string) (*Client, error) {
	tok, err := GenToken(keyFile, keyText, tokenDuration)
	if err != nil {
		return nil, fmt.Errorf("error generating token: %w", err)
	}
	return NewSocketClient(socketFile, tok, proxyTo)
}
