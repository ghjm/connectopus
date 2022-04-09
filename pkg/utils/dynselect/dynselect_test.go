package dynselect

import (
	"context"
	"github.com/ghjm/connectopus/pkg/utils/syncrovar"
	"sync/atomic"
	"testing"
	"time"
)

type funcTester struct {
	good int32
	bad  int32
}

func (t *funcTester) ShouldCall() {
	atomic.AddInt32(&t.good, 1)
}

func (t *funcTester) ShouldNotCall() {
	atomic.AddInt32(&t.bad, 1)
}

func (t *funcTester) Result(desired int32) bool {
	return t.good == desired && t.bad == 0
}

func waitForExit(exited *syncrovar.SyncroVar[bool]) bool {
	sctx, scancel := context.WithTimeout(context.Background(), time.Second)
	defer scancel()
	for {
		time.Sleep(time.Millisecond)
		if exited.Get() {
			return true
		}
		if sctx.Err() != nil {
			return false
		}
	}
}

func TestDynselectSend(t *testing.T) {
	ch1 := make(chan struct{})
	ch2 := make(chan struct{})
	s := &Selector{}
	ft := &funcTester{}
	AddRecv(s, ch1, func(struct{}, bool) {
		ft.ShouldNotCall()
	})
	AddSend(s, ch2, struct{}{}, func() {
		ft.ShouldCall()
	})
	exited := syncrovar.SyncroVar[bool]{}
	go func() {
		s.Select()
		exited.Set(true)
	}()
	<-ch2
	if !waitForExit(&exited) {
		t.Errorf("select call did not return")
	}
	if !ft.Result(1) {
		t.Errorf("select call did not produce expected result")
	}
}

func TestDynselectRecv(t *testing.T) {
	ch1 := make(chan struct{})
	ch2 := make(chan struct{})
	s := &Selector{}
	ft := &funcTester{}
	AddRecv(s, ch1, func(struct{}, bool) {
		ft.ShouldCall()
	})
	AddSend(s, ch2, struct{}{}, func() {
		ft.ShouldNotCall()
	})
	exited := syncrovar.SyncroVar[bool]{}
	go func() {
		s.Select()
		exited.Set(true)
	}()
	ch1 <- struct{}{}
	if !waitForExit(&exited) {
		t.Errorf("select call did not return")
	}
	if !ft.Result(1) {
		t.Errorf("select call did not produce expected result")
	}
}

func TestDynselectDefault(t *testing.T) {
	ch1 := make(chan struct{})
	ch2 := make(chan struct{})
	s := &Selector{}
	ft := &funcTester{}
	AddRecvDiscard(s, ch1, func() {
		ft.ShouldNotCall()
	})
	AddSend(s, ch2, struct{}{}, func() {
		ft.ShouldNotCall()
	})
	AddDefault(s, func() {
		ft.ShouldCall()
	})
	exited := syncrovar.SyncroVar[bool]{}
	go func() {
		s.Select()
		exited.Set(true)
	}()
	if !waitForExit(&exited) {
		t.Errorf("select call did not return")
	}
	if !ft.Result(1) {
		t.Errorf("select call did not produce expected result")
	}
}
