package backend_tcp

import (
	"context"
	"crypto/tls"
	"github.com/ghjm/connectopus/pkg/backends/channel_runner"
	"github.com/ghjm/golib/pkg/makecert"
	"go.uber.org/goleak"
	"net"
	"strconv"
	"testing"
)

func TestBackendTCP(t *testing.T) {
	defer goleak.VerifyNone(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	n1 := channel_runner.NewChannelRunner()
	n2 := channel_runner.NewChannelRunner()
	l := Listener{}
	addr, err := l.Run(ctx, n1)
	if err != nil {
		t.Fatalf("listener backend error %s", err)
	}
	var portStr string
	_, portStr, err = net.SplitHostPort(addr.String())
	if err != nil {
		t.Fatalf("error splitting host:port: %s", err)
	}
	var port int
	port, err = strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("error converting port to integer: %s", err)
	}
	d := Dialer{
		DestAddr: net.ParseIP("127.0.0.1"),
		// #nosec G115
		DestPort: uint16(port),
	}
	err = d.Run(ctx, n2)
	if err != nil {
		t.Fatalf("dialer backend error %s", err)
	}
	go func() {
		n1.WriteChan() <- []byte("hello")
	}()
	data := <-n2.ReadChan()
	if string(data) != "hello" {
		t.Fatalf("incorrect data received")
	}
}

func TestBackendTLS(t *testing.T) {
	defer goleak.VerifyNone(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ca, err := makecert.MakeCA("Test", 1024, 1)
	if err != nil {
		t.Fatal(err)
	}
	var cert *makecert.Cert
	cert, err = ca.MakeCert("Test", 1024, 1, []net.IP{net.ParseIP("127.0.0.1")}, nil)
	if err != nil {
		t.Fatal(err)
	}

	n1 := channel_runner.NewChannelRunner()
	n2 := channel_runner.NewChannelRunner()
	l := Listener{
		TLS: &tls.Config{
			Certificates: []tls.Certificate{cert.TLSCert},
			ClientAuth:   tls.RequireAndVerifyClientCert,
			ClientCAs:    ca.Pool,
			MinVersion:   tls.VersionTLS12,
		},
	}
	var addr net.Addr
	addr, err = l.Run(ctx, n1)
	if err != nil {
		t.Fatalf("listener backend error %s", err)
	}
	var portStr string
	_, portStr, err = net.SplitHostPort(addr.String())
	if err != nil {
		t.Fatalf("error splitting host:port: %s", err)
	}
	var port int
	port, err = strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("error converting port to integer: %s", err)
	}
	d := Dialer{
		DestAddr: net.ParseIP("127.0.0.1"),
		// #nosec G115
		DestPort: uint16(port),
		TLS: &tls.Config{
			Certificates: []tls.Certificate{cert.TLSCert},
			RootCAs:      ca.Pool,
			ServerName:   "127.0.0.1",
			MinVersion:   tls.VersionTLS12,
		},
	}
	err = d.Run(ctx, n2)
	if err != nil {
		t.Fatalf("dialer backend error %s", err)
	}
	go func() {
		n1.WriteChan() <- []byte("hello")
	}()
	data := <-n2.ReadChan()
	if string(data) != "hello" {
		t.Fatalf("incorrect data received")
	}
}
