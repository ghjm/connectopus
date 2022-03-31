package backends

import (
	"context"
	"os"
	"time"
)

type channelRunner struct {
	readChan chan []byte
	writeChan chan []byte
}

// NewChannelRunner returns a ProtocolRunner that just connects channels.  Used for testing backends.
func NewChannelRunner() *channelRunner {
	return &channelRunner{
		readChan:  make(chan []byte),
		writeChan: make(chan []byte),
	}
}

func (p *channelRunner) ReadChan() <-chan []byte {
	return p.readChan
}

func (p *channelRunner) WriteChan() chan<- []byte {
	return p.writeChan
}

func (p *channelRunner) RunProtocol(ctx context.Context, conn BackendConnection) {
	protoCtx, protoCancel := context.WithCancel(ctx)
	defer protoCancel()
	go func() {
		for {
			_ = conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			data, err := conn.ReadMessage()
			if protoCtx.Err() != nil {
				return
			}
			if err == os.ErrDeadlineExceeded {
				continue
			} else if err != nil {
				return
			}
			select {
			case <-protoCtx.Done():
				return
			case p.readChan<-data:
			}
		}
	}()
	go func() {
		for {
			select {
			case <-protoCtx.Done():
				return
			case data := <-p.writeChan:
				for {
					_ = conn.SetWriteDeadline(time.Now().Add(100 * time.Millisecond))
					err := conn.WriteMessage(data)
					if protoCtx.Err() != nil {
						return
					}
					if err == os.ErrDeadlineExceeded {
						continue
					} else if err != nil {
						return
					}
				}
			}
		}
	}()
	<-protoCtx.Done()
}
