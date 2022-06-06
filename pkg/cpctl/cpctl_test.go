package cpctl

import (
	"context"
	"go.uber.org/goleak"
	"io/ioutil"
	"os"
	"testing"
)

func TestCpctl(t *testing.T) {
	defer goleak.VerifyNone(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := Resolver{}
	f, err := ioutil.TempFile("", "temp_cpctl_*.sock")
	if err != nil {
		t.Fatal(err)
	}
	fn := f.Name()
	defer func() {
		_ = os.Remove(fn)
	}()
	err = os.Remove(fn)
	if err != nil {
		t.Fatal(err)
	}
	err = r.ServeUnix(ctx, fn)
	if err != nil {
		t.Fatal(err)
	}
	var c *Client
	c, err = NewSocketClient(fn)
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.GetNetns(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
}
