package broker

import (
	"context"
	"go.uber.org/goleak"
	"sync"
	"testing"
	"time"
)

func TestBrokerShutdown(t *testing.T) {
	defer goleak.VerifyNone(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bctx, bcancel := context.WithCancel(ctx)
	b := New[string](bctx)
	noRecCh := b.Subscribe()
	go func() {
		for {
			select {
			case _, ok := <-noRecCh:
				if ok {
					t.Errorf("message should not have been received")
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	bcancel()
	time.Sleep(10 * time.Millisecond)

	// Test that sending to a terminated broker is a no-op
	b.Publish("foo")

	// Test that subscribing to a terminated broker returns nil
	termSub := b.Subscribe()
	if termSub != nil {
		t.Errorf("subscribe to terminated broker returned a real channel")
	}
}

func TestBroker(t *testing.T) {
	defer goleak.VerifyNone(t)
	testmsg := "hello"
	numSubs := 100
	numToSend := 100
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b := New[string](ctx)
	subCh := make([]<-chan string, numSubs)
	for i := 0; i < numSubs; i++ {
		subCh[i] = b.Subscribe()
	}
	recv := sync.Map{}
	sendToBroker := func(numSubsInUse int) {
		wg := &sync.WaitGroup{}
		wg.Add(numSubs)
		for i := 0; i < numSubs; i++ {
			go func(id int, wg *sync.WaitGroup, c <-chan string) {
				defer wg.Done()
				count := 0
				for {
					timer := time.NewTimer(250 * time.Millisecond)
					select {
					case s, ok := <-c:
						if !ok {
							timer.Stop()
							return
						}
						if s != testmsg {
							t.Errorf("wrong message received: id %d expecting %s, got %s", id, testmsg, s)
							timer.Stop()
							return
						}
						count++
						recv.Store(id, count)
					case <-timer.C:
						return
					}
				}
			}(i, wg, subCh[i])
		}
		for i := 0; i < numToSend; i++ {
			b.Publish(testmsg)
		}
		wg.Wait()
		for i := 0; i < numSubs; i++ {
			r, ok1 := recv.Load(i)
			ri, ok2 := r.(int)
			if i < numSubsInUse {
				if !ok1 {
					t.Errorf("no result from channel")
				}
				if !ok2 {
					t.Errorf("channel result not int")
				}
				if ri != numToSend {
					t.Errorf("subscribed channel got %d messages, expecting %d", ri, numToSend)
				}
			} else if ok1 {
				if !ok2 {
					t.Errorf("channel result not int")
				}
				if ri > 0 {
					t.Errorf("unsubscribed channel got a message")
				}
			}
		}
	}
	sendToBroker(numSubs)
	recv = sync.Map{}
	for i := numSubs / 2; i < numSubs; i++ {
		b.Unsubscribe(subCh[i])
	}
	sendToBroker(numSubs / 2)
	cancel()
}
