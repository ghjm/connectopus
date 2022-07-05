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
	isServer      bool
}

func (b *pairBackend) MTU() int {
	return b.mtu
}

func (b *pairBackend) WriteMessage(data []byte) error {
	if len(data) > b.mtu {
		return fmt.Errorf("exceeds MTU")
	}
	wd := b.writeDeadline
	var timeChan <-chan time.Time
	if !wd.IsZero() {
		timer := time.NewTimer(time.Until(wd))
		defer timer.Stop()
		timeChan = timer.C
	}
	select {
	case b.sendChan <- data:
	case <-b.ctx.Done():
		return os.ErrClosed
	case <-timeChan:
		return os.ErrDeadlineExceeded
	}
	return nil
}

func (b *pairBackend) ReadMessage() ([]byte, error) {
	rd := b.readDeadline
	var timeChan <-chan time.Time
	if !rd.IsZero() {
		timer := time.NewTimer(time.Until(rd))
		defer timer.Stop()
		timeChan = timer.C
	}
	var data []byte
	select {
	case data = <-b.readChan:
	case <-b.ctx.Done():
		return nil, fmt.Errorf("operation cancelled")
	case <-timeChan:
		return nil, os.ErrDeadlineExceeded
	}
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

func (b *pairBackend) IsServer() bool {
	return b.isServer
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
		readDeadline:  time.Time{},
		sendChan:      pair1to2chan,
		writeDeadline: time.Time{},
		isServer:      true,
	}
	pair2 := &pairBackend{
		ctx:           pairCtx,
		cancel:        pairCancel,
		mtu:           mtu,
		readChan:      pair1to2chan,
		readDeadline:  time.Time{},
		sendChan:      pair2to1chan,
		writeDeadline: time.Time{},
		isServer:      false,
	}
	go pr1.RunProtocol(pairCtx, 1.0, pair1)
	go pr2.RunProtocol(pairCtx, 1.0, pair2)
	return nil
}
