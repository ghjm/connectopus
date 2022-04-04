package pubsub

import (
	"context"
	"go.uber.org/goleak"
	"sync"
	"testing"
	"time"
)

func runTest(t *testing.T, mods ...func(*PublishModifier)) {
	testmsg := "hello"
	numSubs := 100 // must be even
	numToSend := 100
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b := NewBroker[string](ctx)
	subCh := make([]<-chan string, numSubs)
	for i := 0; i < numSubs; i++ {
		subCh[i] = b.Subscribe()
	}
	recv := sync.Map{}
	sendToBroker := func(numSubsInUse int) {
		wg := &sync.WaitGroup{}
		wg.Add(numSubs + 1)
		for i := 0; i < numSubs; i++ {
			go func(id int, wg *sync.WaitGroup, c <-chan string) {
				defer wg.Done()
				count := 0
				for {
					select {
					case s, ok := <-c:
						if !ok {
							return
						}
						if s != testmsg {
							t.Errorf("wrong message received: id %d expecting %s, got %s", id, testmsg, s)
							return
						}
						count++
						recv.Store(id, count)
					case <-time.After(50 * time.Millisecond):
						return
					}
				}
			}(i, wg, subCh[i])
		}
		go func() {
			for i := 0; i < numToSend; i++ {
				b.Publish(testmsg, mods...)
			}
			wg.Done()
		}()
		wg.Wait()
		for i := 0; i < numSubs; i++ {
			r, ok1 := recv.Load(i)
			ri, ok2 := r.(int)
			if i < numSubsInUse {
				if !ok1 {
					t.Fatal("no result from channel")
				}
				if !ok2 {
					t.Fatal("channel result not int")
				}
				if ri != numToSend {
					t.Fatal("subscribed channel got wrong number of messages")
				}
			} else if ok1 {
				if !ok2 {
					t.Fatal("channel result not int")
				}
				if ri > 0 {
					t.Fatal("unsubscribed channel got a message")
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
}

func TestPubSub(t *testing.T) {
	defer goleak.VerifyNone(t)
	runTest(t)
	runTest(t, NoWait)
}
