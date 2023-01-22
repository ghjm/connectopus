package accept_loop

import (
	"context"
	log "github.com/sirupsen/logrus"
	"net"
	"time"
)

func AcceptLoop(ctx context.Context, li net.Listener, connFunc func(context.Context, net.Conn)) {
	var tempDelay time.Duration
	for {
		conn, err := li.Accept()
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			log.Warnf("accept error: %s; retrying in %v", err, tempDelay)
			if tempDelay == 0 {
				tempDelay = 5 * time.Millisecond
			} else {
				tempDelay *= 2
			}
			if max := 1 * time.Second; tempDelay > max {
				tempDelay = max
			}
			time.Sleep(tempDelay)
			continue
		}
		tempDelay = 0
		go func() {
			connFunc(ctx, conn)
		}()
	}
}
