package backends

import (
	"context"
	"github.com/ghjm/connectopus/pkg/utils/syncro"
	"go.uber.org/goleak"
	"os"
	"testing"
	"time"
)

var testMsg = "Hello, world!"

// implements BackendConnection
type testBackend struct {
	ctx          context.Context
	msgSent      bool
	readDeadline time.Time
	t            *testing.T
	gotWrite     syncro.Var[bool]
}

func (b *testBackend) MTU() int {
	return 1500
}

func (b *testBackend) WriteMessage(data []byte) error {
	if string(data) != testMsg {
		b.t.Errorf("invalid data written. expected %s, got %s", testMsg, data)
	}
	b.gotWrite.Set(true)
	return nil
}

func (b *testBackend) ReadMessage() ([]byte, error) {
	if !b.msgSent {
		b.msgSent = true
		return []byte(testMsg), nil
	}
	timer := time.NewTimer(b.readDeadline.Sub(time.Now()))
	defer timer.Stop()
	select {
	case <-b.ctx.Done():
	case <-timer.C:
	}
	return nil, os.ErrDeadlineExceeded
}

func (b *testBackend) Close() error {
	return nil
}

func (b *testBackend) SetReadDeadline(t time.Time) error {
	b.readDeadline = t
	return nil
}

func (b *testBackend) SetWriteDeadline(_ time.Time) error {
	return nil
}

func TestChannelRunner(t *testing.T) {
	defer goleak.VerifyNone(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b := &testBackend{
		ctx: ctx,
		t:   t,
	}
	c := NewChannelRunner()

	gotRead := syncro.Var[bool]{}
	go func() {
		select {
		case <-ctx.Done():
		case msg := <-c.ReadChan():
			if string(msg) != testMsg {
				t.Errorf("invalid message: expected %s, got %s", testMsg, msg)
			}
			gotRead.Set(true)
		}
	}()

	go func() {
		select {
		case <-ctx.Done():
		case c.WriteChan() <- []byte(testMsg):
		}
	}()

	go c.RunProtocol(ctx, b)

	startTime := time.Now()
	for {
		if (gotRead.Get() && b.gotWrite.Get()) || time.Now().Sub(startTime) > 5*time.Second {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !gotRead.Get() {
		t.Fatalf("did not read any data")
	}
	if !b.gotWrite.Get() {
		t.Fatalf("did not write any data")
	}
}
