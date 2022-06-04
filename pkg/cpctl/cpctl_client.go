package cpctl

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"time"
)

func NewSocketClient(socketFile string) (*Client, error) {
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
	return NewClient(client, "http://cpctl.sock/query"), nil
}
