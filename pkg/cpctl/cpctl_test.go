//go:build !windows

package cpctl

import (
	"context"
	"github.com/ghjm/connectopus/pkg/config"
	"go.uber.org/goleak"
	"io/ioutil"
	"os"
	"testing"
	"time"
)

func TestCpctl(t *testing.T) {
	defer goleak.VerifyNone(t)
	f, err := ioutil.TempFile("", "temp_cpctl_*.sock")
	if err != nil {
		t.Fatal(err)
	}
	fn := f.Name()
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
		time.Sleep(50 * time.Millisecond)
		_ = os.Remove(fn)
	}()
	r := Server{
		Resolver: Resolver{
			C: &config.Config{
				Global: config.Global{
					AuthorizedKeys: nil,
				},
			},
		},
		SigningMethod: nil,
	}
	err = os.Remove(fn)
	if err != nil {
		t.Fatal(err)
	}
	err = r.ServeUnix(ctx, fn)
	if err != nil {
		t.Fatal(err)
	}
	var c *Client
	c, err = NewSocketClient(fn, "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.GetNetns(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
}
