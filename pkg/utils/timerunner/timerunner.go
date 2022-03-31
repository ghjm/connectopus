package timerunner

import (
	"context"
	"github.com/ghjm/connectopus/pkg/utils"
	"github.com/ghjm/connectopus/pkg/utils/modifiers"
	"time"
)

type TimeRunner interface {
	RunWithin(t time.Duration)
}

type timerunner struct {
	ctx      context.Context
	nextRun  time.Time
	reqChan  chan time.Duration
	f        func()
	periodic time.Duration
	nowait   bool
}

// Periodic modifies NewTimeRunner to include periodic activations
func Periodic(period time.Duration) func(*timerunner) {
	return func(tr *timerunner) {
		tr.periodic = period
	}
}

// AtStart modifies NewTimeRunner to run the function once immediately at startup
func AtStart(tr *timerunner) {
	tr.nextRun = time.Now()
}

// NoWait modifies NewTimeRunner to run the function in a goroutine.  This means multiple instances
// of the function could run simultaneously if it is slow enough, and also that calls to RunWithin don't block.
func NoWait(tr *timerunner) {
	tr.nowait = true
}

// NewTimeRunner returns a new TimeRunner which will execute function f at appropriate times
func NewTimeRunner(ctx context.Context, f func(), mods... func(*timerunner)) TimeRunner {
	tr := &timerunner{
		ctx:      ctx,
		nextRun:  utils.TimeNever,
		reqChan:  make(chan time.Duration),
		f:        f,
		periodic: utils.TimeNever.Sub(time.Time{}),
	}
	modifiers.ProcessMods(tr, mods)
	go tr.mainLoop()
	return tr
}

func (tr *timerunner) mainLoop() {
	for {
		select {
		case <-tr.ctx.Done():
			return
		case <-time.After(tr.nextRun.Sub(time.Now())):
			if tr.nowait {
				go tr.f()
			} else {
				tr.f()
			}
			tr.nextRun = time.Now().Add(tr.periodic)
		case timeReq := <-tr.reqChan:
			reqNext := time.Now().Add(timeReq)
			if reqNext.Before(tr.nextRun) {
				tr.nextRun = reqNext
			}
		}
	}
}

// RunWithin requests that the TimeRunner execute within a given duration
func (tr *timerunner) RunWithin(t time.Duration) {
	tr.reqChan <- t
}
