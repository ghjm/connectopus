package timerunner

import (
	"context"
	"github.com/ghjm/connectopus/pkg/x/dynselect"
	"github.com/ghjm/connectopus/pkg/x/modifiers"
	"time"
)

type TimeRunner interface {
	RunWithin(t time.Duration)
}

type timerunner struct {
	ctx      context.Context
	nextRun  *time.Time
	reqChan  chan time.Duration
	f        func()
	periodic *time.Duration
	events   []func(*dynselect.Selector)
}

// Periodic modifies New to include periodic activations
func Periodic(period time.Duration) func(*timerunner) {
	return func(tr *timerunner) {
		tr.periodic = &period
	}
}

// AtStart modifies New to run the function once immediately at startup
func AtStart(tr *timerunner) {
	tn := time.Now()
	tr.nextRun = &tn
}

// EventChan modifies New to include a new event channel.  The point of this is to be able to run
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

// New returns a new TimeRunner which will execute function f at appropriate times
func New(ctx context.Context, f func(), mods ...func(*timerunner)) TimeRunner {
	tr := &timerunner{
		ctx:      ctx,
		nextRun:  nil,
		reqChan:  make(chan time.Duration),
		f:        f,
		periodic: nil,
	}
	modifiers.ProcessMods(tr, mods)
	if tr.periodic != nil && tr.nextRun == nil {
		nr := time.Now().Add(*tr.periodic)
		tr.nextRun = &nr
	}
	go tr.mainLoop(ctx)
	return tr
}

func (tr *timerunner) mainLoop(ctx context.Context) {
	for {
		shouldReturn := false

		// Construct timer till next known event
		var timer *time.Timer
		tn := time.Now()
		if tr.nextRun != nil {
			delayTime := time.Millisecond
			if tn.Before(*tr.nextRun) {
				delayTime = tr.nextRun.Sub(tn)
			}
			timer = time.NewTimer(delayTime)
		}

		// dynamic select
		s := &dynselect.Selector{}
		// case <-ctx.Done()
		dynselect.AddRecvDiscard(s, ctx.Done(), func() {
			shouldReturn = true
		})
		// case <-timer.C
		if timer != nil {
			dynselect.AddRecvDiscard(s, timer.C, func() {
				tr.f()
				if tr.periodic != nil {
					nr := time.Now().Add(*tr.periodic)
					tr.nextRun = &nr
				} else {
					tr.nextRun = nil
				}
			})
		}
		// case <-tr.reqChan
		dynselect.AddRecv(s, tr.reqChan, func(timeReq time.Duration, _ bool) {
			reqNext := time.Now().Add(timeReq)
			if tr.nextRun == nil || reqNext.Before(*tr.nextRun) {
				tr.nextRun = &reqNext
			}
		})
		// Add custom events to the selector
		for _, e := range tr.events {
			e(s)
		}
		s.Select()
		if timer != nil {
			timer.Stop()
		}
		if shouldReturn {
			return
		}
	}
}

// RunWithin requests that the TimeRunner execute within a given duration
func (tr *timerunner) RunWithin(t time.Duration) {
	go func() {
		// this is run in a goroutine to avoid deadlocking when called inside an event handler
		select {
		case <-tr.ctx.Done():
			return
		case tr.reqChan <- t:
		}
	}()
}
