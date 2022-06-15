package channel_runner

import (
	"context"
	"github.com/ghjm/connectopus/pkg/backends"
	"os"
	"time"
)

// channelRunner implements ProtocolRunner
type channelRunner struct {
	readChan  chan []byte
	writeChan chan []byte
}

// NewChannelRunner returns a ProtocolRunner that just exposes read and write channels.  This allows
// low-level testing of backends by just pushing datagrams through them, without using Netopus at all.
func NewChannelRunner() *channelRunner {
	return &channelRunner{
		readChan:  make(chan []byte),
		writeChan: make(chan []byte),
	}
}

// ReadChan returns a channel that messages from the backend can be read from.
func (p *channelRunner) ReadChan() <-chan []byte {
	return p.readChan
}

// WriteChan returns a channel that messages to the backend can be written to.
func (p *channelRunner) WriteChan() chan<- []byte {
	return p.writeChan
}

func (p *channelRunner) RunProtocol(ctx context.Context, cost float32, conn backends.BackendConnection) {
	protoCtx, protoCancel := context.WithCancel(ctx)

	// Goroutine that reads from conn and writes to readChan
	go func() {
		defer protoCancel()
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
			case p.readChan <- data:
			}
		}
	}()

	// Goroutine that reads from writeChan and writes to conn
	go func() {
		defer protoCancel()
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
