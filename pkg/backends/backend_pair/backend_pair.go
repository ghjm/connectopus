package backend_pair

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/pkg/backends"
	"os"
	"time"
)

// implements BackendConnection
type pairBackend struct {
	ctx           context.Context
	cancel        context.CancelFunc
	mtu           int
	readChan      chan []byte
	readDeadline  time.Time
	sendChan      chan []byte
	writeDeadline time.Time
}

var timeNever = time.Date(9999, 0, 0, 0, 0, 0, 0, time.UTC)

func (b *pairBackend) MTU() int {
	return b.mtu
}

func (b *pairBackend) WriteMessage(data []byte) error {
	if len(data) > b.mtu {
		return fmt.Errorf("exceeds MTU")
	}
	wd := b.writeDeadline
	if wd.IsZero() {
		wd = timeNever
	}
	timer := time.NewTimer(time.Until(wd))
	defer timer.Stop()
	select {
	case b.sendChan <- data:
	case <-b.ctx.Done():
		return os.ErrClosed
	case <-timer.C:
		return os.ErrDeadlineExceeded
	}
	return nil
}

func (b *pairBackend) ReadMessage() ([]byte, error) {
	rd := b.readDeadline
	if rd.IsZero() {
		rd = timeNever
	}
	var data []byte
	timer := time.NewTimer(time.Until(rd))
	select {
	case data = <-b.readChan:
	case <-b.ctx.Done():
		timer.Stop()
		return nil, fmt.Errorf("operation cancelled")
	case <-timer.C:
		return nil, os.ErrDeadlineExceeded
	}
	timer.Stop()
	return data, nil
}

func (b *pairBackend) Close() error {
	b.cancel()
	return nil
}

func (b *pairBackend) SetReadDeadline(t time.Time) error {
	b.readDeadline = t
	return nil
}

func (b *pairBackend) SetWriteDeadline(t time.Time) error {
	b.writeDeadline = t
	return nil
}

func RunPair(ctx context.Context, pr1 backends.ProtocolRunner, pr2 backends.ProtocolRunner, mtu int) error {
	pairCtx, pairCancel := context.WithCancel(ctx)
	pair1to2chan := make(chan []byte)
	pair2to1chan := make(chan []byte)
	pair1 := &pairBackend{
		ctx:           pairCtx,
		cancel:        pairCancel,
		mtu:           mtu,
		readChan:      pair2to1chan,
		readDeadline:  timeNever,
		sendChan:      pair1to2chan,
		writeDeadline: timeNever,
	}
	pair2 := &pairBackend{
		ctx:           pairCtx,
		cancel:        pairCancel,
		mtu:           mtu,
		readChan:      pair1to2chan,
		readDeadline:  timeNever,
		sendChan:      pair2to1chan,
		writeDeadline: timeNever,
	}
	go pr1.RunProtocol(pairCtx, pair1)
	go pr2.RunProtocol(pairCtx, pair2)
	return nil
}
