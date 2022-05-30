package syncro

import (
	"context"
	"go.uber.org/goleak"
	"testing"
	"time"
)

func TestSyncroVar(t *testing.T) {
	defer goleak.VerifyNone(t)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	sv := Var[int]{}
	go func() {
		i := 0
		for {
			sv.Set(i)
			i++
			if ctx.Err() != nil {
				return
			}
		}
	}()
	go func() {
		i := 0
		for {
			v := sv.Get()
			if ctx.Err() != nil {
				return
			}
			if v < i {
				t.Errorf("var went backwards")
			}
			i = v
		}
	}()
	<-ctx.Done()
}

func TestSyncroWork(t *testing.T) {
	defer goleak.VerifyNone(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	sv := Var[int]{}
	sv.Set(0)
	go func() {
		for {
			sv.WorkWith(func(i *int) {
				*i++
				time.Sleep(time.Millisecond)
				*i++
				time.Sleep(time.Millisecond)
			})
			if ctx.Err() != nil {
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()
	go func() {
		count := 0
		for {
			v := sv.Get()
			count++
			if ctx.Err() != nil {
				if count < 1000 {
					t.Errorf("Get only succeeded %d times", count)
				}
				return
			}
			if v%2 != 0 {
				t.Errorf("var was odd")
			}
		}
	}()
	<-ctx.Done()
}
