package netopus

import (
	"context"
	"github.com/ghjm/connectopus/pkg/backends/backend_pair"
	"go.uber.org/goleak"
	"net"
	"testing"
	"time"
)

func TestNetopus(t *testing.T) {
	defer goleak.VerifyNone(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	n1, err := NewNetopus(ctx, net.ParseIP("FD00::1"))
	if err != nil {
		t.Fatalf("netopus initialization error %s", err)
	}
	n2, err := NewNetopus(ctx, net.ParseIP("FD00::2"))
	if err != nil {
		t.Fatalf("netopus initialization error %s", err)
	}
	err = backend_pair.RunPair(ctx, n1, n2, 1500)
	if err != nil {
		t.Fatalf("pair backend error %s", err)
	}
	time.Sleep(2 * time.Second)
}
