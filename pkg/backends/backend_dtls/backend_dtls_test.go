package backend_dtls

import (
	"context"
	"github.com/ghjm/connectopus/pkg/backends/channel_runner"
	"go.uber.org/goleak"
	"net"
	"strconv"
	"testing"
)

func TestBackendDtls(t *testing.T) {
	defer goleak.VerifyNone(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	n1 := channel_runner.NewChannelRunner()
	n2 := channel_runner.NewChannelRunner()
	addr, err := RunListener(ctx, n1, 1.00, 0)
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
	err = RunDialer(ctx, n2, 1.0, net.ParseIP("127.0.0.1"), uint16(port))
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
