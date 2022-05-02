package backend_dtls

import (
	"context"
	"github.com/ghjm/connectopus/pkg/backends"
	"go.uber.org/goleak"
	"net"
	"strconv"
	"testing"
)

func TestBackendDtls(t *testing.T) {
	defer goleak.VerifyNone(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	n1 := backends.NewChannelRunner()
	n2 := backends.NewChannelRunner()
	addr, err := RunListener(ctx, n1, 0)
	if err != nil {
		t.Fatalf("listener backend error %s", err)
	}
	var portStr string
	_, portStr, err = net.SplitHostPort(addr.String())
	if err != nil {
		t.Fatalf("error splitting host:port: %s", err)
	}
	var port uint64
	port, err = strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		t.Fatalf("non-numeric port: %s", portStr)
	}
	err = RunDialer(ctx, n2, net.ParseIP("127.0.0.1"), int(port))
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
