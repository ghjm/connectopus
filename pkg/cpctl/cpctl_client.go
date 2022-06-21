package cpctl

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"
)

func NewSocketClient(socketFile string, proxyTo string) (*Client, error) {
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
	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}
	return NewClient(client, clientURL), nil
}
