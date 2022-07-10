package backend_dtls

import (
	"context"
	"github.com/ghjm/connectopus/pkg/backends/channel_runner"
	"github.com/ghjm/connectopus/pkg/x/makecert"
	"go.uber.org/goleak"
	"net"
	"strconv"
	"testing"
	"time"
)

func TestBackendDtlsPSK(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreTopFunction("internal/poll.runtime_pollWait"), goleak.IgnoreTopFunction("sync.runtime_Semacquire"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	n1 := channel_runner.NewChannelRunner()
	n2 := channel_runner.NewChannelRunner()
	l := Listener{
		PSK: "test-psk",
	}
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
		DestPort: uint16(port),
		PSK:      "test-psk",
	}
	err = d.Run(ctx, n2)
	if err != nil {
		t.Fatalf("dialer backend error %s", err)
	}
	go func() {
		select {
		case <-ctx.Done():
		case n1.WriteChan() <- []byte("hello"):
		}
	}()
	var data []byte
	select {
	case <-ctx.Done():
	case data = <-n2.ReadChan():
	}
	if string(data) != "hello" && ctx.Err() == nil {
		t.Fatalf("incorrect data received")
	}
}

func TestBackendDtlsCerts(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreTopFunction("internal/poll.runtime_pollWait"), goleak.IgnoreTopFunction("sync.runtime_Semacquire"))
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
		time.Sleep(250 * time.Millisecond)
	}()

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
		RequireClientCert: true,
		ClientCAs:         ca.Pool,
		Certificate:       cert.TLSCert,
	}
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
		DestAddr:   net.ParseIP("127.0.0.1"),
		DestPort:   uint16(port),
		RootCAs:    ca.Pool,
		ClientCert: &cert.TLSCert,
	}
	err = d.Run(ctx, n2)
	if err != nil {
		t.Fatalf("dialer backend error %s", err)
	}
	go func() {
		select {
		case <-ctx.Done():
		case n1.WriteChan() <- []byte("hello"):
		}
	}()
	var data []byte
	select {
	case <-ctx.Done():
	case data = <-n2.ReadChan():
	}
	if string(data) != "hello" && ctx.Err() == nil {
		t.Fatalf("incorrect data received")
	}
}
