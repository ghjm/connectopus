package backend_pair

import (
	"context"
	"github.com/ghjm/connectopus/pkg/backends/channel_runner"
	"go.uber.org/goleak"
	"testing"
)

func TestBackendPair(t *testing.T) {
	defer goleak.VerifyNone(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	n1 := channel_runner.NewChannelRunner()
	n2 := channel_runner.NewChannelRunner()
	err := RunPair(ctx, n1, n2, 1500)
	if err != nil {
		t.Fatalf("pair backend error %s", err)
	}
	go func() {
		n1.WriteChan() <- []byte("hello")
	}()
	data := <-n2.ReadChan()
	if string(data) != "hello" {
		t.Fatalf("incorrect data received")
	}
}
