package cpctl

import (
	"context"
	"errors"
	"fmt"
	"github.com/Yamashou/gqlgenc/clientv2"
	"github.com/ghjm/connectopus/pkg/config"
	"github.com/ghjm/golib/pkg/ssh_jwt"
	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/crypto/ssh/agent"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path"
	"time"
)

var ErrNoSSHKeys = fmt.Errorf("no SSH keys found")
var ErrMultipleSSHKeys = fmt.Errorf("multiple SSH keys found; use --text to select one uniquely")

func GetUsableKey(a agent.Agent, keyText string) (string, error) {
	keys, err := ssh_jwt.GetMatchingKeys(a, keyText)
	if err != nil {
		return "", fmt.Errorf("error listing SSH keys: %w", err)
	}
	if len(keys) == 0 {
		return "", ErrNoSSHKeys
	}
	if len(keys) > 1 {
		return "", ErrMultipleSSHKeys
	}
	return keys[0].String(), nil
}

// GenTokenFromAgent generates a token from an already-open agent using a key matching keyText
func GenTokenFromAgent(a agent.Agent, keyText string, tokenDuration string) (string, error) {
	key, err := GetUsableKey(a, keyText)
	if err != nil {
		return "", fmt.Errorf("error getting SSH key: %w", err)
	}
	var sm jwt.SigningMethod
	sm, err = ssh_jwt.SetupSigningMethod("connectopus", a)
	if err != nil {
		return "", fmt.Errorf("error initializing JWT signing method: %w", err)
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
				return "", fmt.Errorf("error parsing token duration: %w", err)
			}
		}
		claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(expireDuration))
	}
	tok := jwt.NewWithClaims(sm, claims)
	tok.Header["kid"] = key
	var tokstr string
	tokstr, err = tok.SignedString(key)
	if err != nil {
		return "", fmt.Errorf("error signing token: %w", err)
	}
	return tokstr, nil
}

// GenToken generates a token by either communicating with an agent, or loading the key from keyFile.
func GenToken(keyFile string, keyText string, tokenDuration string) (string, error) {
	a, err := ssh_jwt.GetSSHAgent(keyFile)
	if err != nil {
		return "", fmt.Errorf("error initializing SSH agent: %w", err)
	}
	defer func() {
		_ = a.Close()
	}()
	return GenTokenFromAgent(a, keyText, tokenDuration)
}

var ErrNoNode = fmt.Errorf("no running node found")
var ErrMultipleNode = fmt.Errorf("multiple running nodes found")

func FindSockets(socketNode string) ([]string, error) {
	dataDirs, err := config.FindDataDirs(socketNode)
	if err != nil {
		return nil, err
	}
	var socketFiles []string
	for _, dir := range dataDirs {
		fn := path.Join(dir, "cpctl.sock")
		var fi os.FileInfo
		fi, err = os.Stat(fn)
		if err == nil && !fi.IsDir() {
			socketFiles = append(socketFiles, fn)
		}
	}
	return socketFiles, nil
}

func FindSocket(socketNode string) (string, error) {
	envSock := os.Getenv("CPCTL_SOCK")
	if envSock != "" {
		return envSock, nil
	}
	socketList, err := FindSockets(socketNode)
	if err != nil {
		return "", fmt.Errorf("error listing socket files: %w", err)
	}
	if len(socketList) == 0 {
		return "", ErrNoNode
	}
	if len(socketList) > 1 {
		return "", ErrMultipleNode
	}
	return socketList[0], nil
}

// NewSocketClient creates a new socket client using a given auth token
func NewSocketClient(socketFile string, socketNode string, authToken string, proxyTo string) (*Client, error) {
	if socketFile == "" {
		var err error
		socketFile, err = FindSocket(socketNode)
		if err != nil {
			return nil, err
		}
	}

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
	return NewClient(client, clientURL, &clientv2.Options{}), nil
}

// NewTokenAndSocketClient generates a token and then uses it to create a new socket client
func NewTokenAndSocketClient(socketFile string, socketNode string, keyFile string, keyText string, tokenDuration string, proxyTo string) (*Client, error) {
	tok, err := GenToken(keyFile, keyText, tokenDuration)
	if err != nil {
		return nil, fmt.Errorf("error generating token: %w", err)
	}
	return NewSocketClient(socketFile, socketNode, tok, proxyTo)
}
