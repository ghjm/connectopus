package utils

import (
	"context"
	log "github.com/sirupsen/logrus"
	"time"
)

var TimeNever = time.Date(9999, 0, 0, 0, 0, 0, 0, time.UTC)

func LogTime(ctx context.Context, descr string, incr time.Duration) context.CancelFunc {
	timeCtx, cancel := context.WithCancel(ctx)
	go func() {
		startTime := time.Now()
		for {
			select {
			case <-timeCtx.Done():
				return
			case <-time.After(incr):
				log.Debugf("waiting for %s: %v\n", descr, time.Now().Sub(startTime))
			}
		}
	}()
	return cancel
}
