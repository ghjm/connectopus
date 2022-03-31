package backend_dtls

import (
	"context"
	"github.com/ghjm/connectopus/pkg/backends"
	"go.uber.org/goleak"
	"net"
	"testing"
)

func TestBackendDtls(t *testing.T) {
	defer goleak.VerifyNone(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	n1 := backends.NewChannelRunner()
	n2 := backends.NewChannelRunner()
	err := RunListener(ctx, n1, 5691)
	if err != nil {
		t.Fatalf("listener backend error %s", err)
	}
	err = RunDialer(ctx, n2, net.ParseIP("127.0.0.1"), 5691)
	if err != nil {
		t.Fatalf("dialer backend error %s", err)
	}
	go func() {
		n1.WriteChan() <- []byte("hello")
	}()
	data := <- n2.ReadChan()
	if string(data) != "hello" {
		t.Fatalf("incorrect data received")
	}
}

