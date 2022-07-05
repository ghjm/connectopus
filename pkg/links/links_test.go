//go:build linux

package links

import (
	"context"
	"github.com/ghjm/connectopus/pkg/x/chanreader"
	"github.com/ghjm/connectopus/pkg/x/syncro"
	"go.uber.org/goleak"
	"io"
	"testing"
	"time"
)

type dummyRWC struct {
	data   syncro.Var[[]string]
	closed syncro.Var[bool]
}

func (d *dummyRWC) Read(p []byte) (int, error) {
	var readData *string
	for {
		if d.closed.Get() {
			return 0, io.ErrClosedPipe
		}
		d.data.WorkWith(func(data *[]string) {
			if len(*data) > 0 {
				dStr := (*data)[0]
				readData = &dStr
				*data = (*data)[1:]
			}
		})
		if readData != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	copy(p, *readData)
	return len(*readData), nil
}

func (d *dummyRWC) Write(p []byte) (int, error) {
	d.data.WorkWith(func(data *[]string) {
		*data = append(*data, string(p))
	})
	return len(p), nil
}

func (d *dummyRWC) Close() error {
	d.closed.Set(true)
	return nil
}

type dummyLink struct {
	chanreader.Publisher
	rwc io.ReadWriteCloser
}

func (d *dummyLink) SendPacket(packet []byte) error {
	_, err := d.rwc.Write(packet)
	if err != nil {
		return err
	}
	return nil
}

func NewDummyLink(ctx context.Context) Link {
	rwc := &dummyRWC{}
	l := &dummyLink{
		Publisher: *chanreader.NewPublisher(ctx, rwc, chanreader.WithBufferSize(1500)),
		rwc:       rwc,
	}
	return l
}

var testData = []string{
	"Hello",
	"Goodbye",
	"I",
	"Am",
	"The",
	"Walrus",
}

func TestLink(t *testing.T) {
	defer goleak.VerifyNone(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var inCount syncro.Var[int]
	dl := NewDummyLink(ctx)
	packChan := dl.SubscribePackets()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case packet := <-packChan:
				inCount.WorkWith(func(i *int) {
					if string(packet) != testData[*i] {
						t.Errorf("wrong packet data received")
					}
					*i++
				})
			}
		}
	}()
	for _, s := range testData {
		err := dl.SendPacket([]byte(s))
		if err != nil {
			t.Errorf("packet send error %s", err)
		}
	}
	startTime := time.Now()
	for {
		if time.Since(startTime) > time.Second {
			t.Errorf("timed out")
		}
		if inCount.Get() == len(testData) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
}
