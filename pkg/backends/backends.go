package backends

import (
	"context"
	"fmt"
	log "github.com/sirupsen/logrus"
	"strings"
	"time"
)

// BackendConnection represents a single back-end connection.  Message transport need not be reliable,
// but Backends must guarantee that whole messages transit the network intact -  ie, that ReadMessage,
// if it returns anything, always returns the entire message that was provided (on another node) to
// WriteMessage, not just a fragment of it.
type BackendConnection interface {
	MTU() int
	WriteMessage([]byte) error
	ReadMessage() ([]byte, error)
	Close() error
	SetReadDeadline(time.Time) error
	SetWriteDeadline(time.Time) error
}

// ProtocolRunner is called by backends to run a protocol over a connection
type ProtocolRunner interface {
	RunProtocol(context.Context, float32, BackendConnection)
}

var ErrExceedsMTU = fmt.Errorf("payload size exceeds MTU")

// ConnFunc is a type of function that gets a new BackendConnection, such as by dialing or accepting
type ConnFunc func() (BackendConnection, error)

// RunDialer abstractly implements retry logic for repeatedly dialing a peer until it connects
func RunDialer(ctx context.Context, pr ProtocolRunner, cost float32, dialer ConnFunc) {
	var nextTimeout time.Duration
	for {
		if nextTimeout > 0 {
			timer := time.NewTimer(nextTimeout)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
		if nextTimeout == 0 {
			nextTimeout = time.Second
		} else {
			nextTimeout = nextTimeout * 6 / 5
			if nextTimeout > time.Minute {
				nextTimeout = time.Minute
			}
		}
		conn, err := dialer()
		if err != nil {
			log.Debugf("dialer error: %s", err)
			continue
		}
		nextTimeout = 0
		pr.RunProtocol(ctx, cost, conn)
	}
}

// RunListener abstractly implements the accept loop of a listener
func RunListener(ctx context.Context, pr ProtocolRunner, cost float32, acceptor ConnFunc) {
	for {
		conn, err := acceptor()
		if err != nil {
			if !strings.Contains(err.Error(), "listener closed") {
				log.Warnf("accept error: %s", err)
			}
			return
		}
		go pr.RunProtocol(ctx, cost, conn)
	}
}
