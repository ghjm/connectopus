package dynselect

import (
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
	exited := false
	go func() {
		s.Select()
		exited = true
	}()
	_ = <-ch2
	if !exited || !ft.Result(1) {
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
	exited := false
	go func() {
		s.Select()
		exited = true
	}()
	ch1 <- struct{}{}
	if !exited || !ft.Result(1) {
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
	exited := false
	go func() {
		s.Select()
		exited = true
	}()
	time.Sleep(time.Millisecond)
	if !exited || !ft.Result(1) {
		t.Errorf("select call did not produce expected result")
	}
}
