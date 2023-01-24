package proxies

import (
	"context"
	"go.uber.org/goleak"
	"testing"
	"time"
)

func TestProxyTCP(t *testing.T) {
	defer goleak.VerifyNone(t)
	_, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
}

func TestProxyUDP(t *testing.T) {
	defer goleak.VerifyNone(t)
	_, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

}
