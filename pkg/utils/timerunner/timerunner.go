package timerunner

import (
	"context"
	"github.com/ghjm/connectopus/pkg/utils"
	"github.com/ghjm/connectopus/pkg/utils/dynselect"
	"github.com/ghjm/connectopus/pkg/utils/modifiers"
	"time"
)

type TimeRunner interface {
	RunWithin(t time.Duration)
}

type timerunner struct {
	nextRun  time.Time
	reqChan  chan time.Duration
	f        func()
	periodic time.Duration
	nowait   bool
	events   []func(*dynselect.Selector)
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

// EventChan modifies NewTimeRunner to include a new event channel.  The point of this is to be able to run
// your own code within the timerunner's goroutine, to avoid the need for external synchronization like mutexes.
func EventChan[T any](ch <-chan T, f func(T)) func(*timerunner) {
	return func(tr *timerunner) {
		tr.events = append(tr.events, func(s *dynselect.Selector) {
			dynselect.AddRecv(s, ch, func(v T, ok bool) {
				f(v)
			})
		})
	}
}

// NewTimeRunner returns a new TimeRunner which will execute function f at appropriate times
func NewTimeRunner(ctx context.Context, f func(), mods ...func(*timerunner)) TimeRunner {
	tr := &timerunner{
		nextRun:  utils.TimeNever,
		reqChan:  make(chan time.Duration),
		f:        f,
		periodic: utils.TimeNever.Sub(time.Time{}),
	}
	modifiers.ProcessMods(tr, mods)
	go tr.mainLoop(ctx)
	return tr
}

func (tr *timerunner) mainLoop(ctx context.Context) {
	for {
		shouldReturn := false
		s := &dynselect.Selector{}
		dynselect.AddRecvDiscard(s, ctx.Done(), func() { shouldReturn = true })
		dynselect.AddRecvDiscard(s, time.After(tr.nextRun.Sub(time.Now())), func() {
			if tr.nowait {
				go tr.f()
			} else {
				tr.f()
			}
			tr.nextRun = time.Now().Add(tr.periodic)
		})
		dynselect.AddRecv(s, tr.reqChan, func(timeReq time.Duration, _ bool) {
			reqNext := time.Now().Add(timeReq)
			if reqNext.Before(tr.nextRun) {
				tr.nextRun = reqNext
			}
		})
		for _, e := range tr.events {
			e(s)
		}
		s.Select()
		if shouldReturn {
			return
		}
	}
}

// RunWithin requests that the TimeRunner execute within a given duration
func (tr *timerunner) RunWithin(t time.Duration) {
	go func() {
		tr.reqChan <- t // this is run in a goroutine to avoid deadlocking when called inside an event handler
	}()
}
