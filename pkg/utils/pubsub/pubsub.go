package pubsub

import (
	"context"
	"github.com/ghjm/connectopus/pkg/utils/modifiers"
	"gvisor.dev/gvisor/pkg/sync"
)

type Subscriber[T any] interface {
	Subscribe() <-chan T
	Unsubscribe(<-chan T)
}

type PublishModifier struct {
	nowait bool
}

type Publisher[T any] interface {
	Publish(T, ...func(*PublishModifier))
}

type Broker[T any] interface {
	Subscriber[T]
	Publisher[T]
}

type chanMsg[T any] struct {
	data T
	done func()
}

type broker[T any] struct {
	ctx         context.Context
	publishChan chan chanMsg[T]
	subChan     chan chanMsg[chan T]
	unsubChan   chan chanMsg[<-chan T]
}

func NewBroker[T any](ctx context.Context) Broker[T] {
	b := &broker[T]{
		ctx:         ctx,
		publishChan: make(chan chanMsg[T]),
		subChan:     make(chan chanMsg[chan T]),
		unsubChan:   make(chan chanMsg[<-chan T]),
	}
	go b.start()
	return b
}

func (b *broker[T]) start() {
	subs := make(map[chan T]struct{})
	for {
		select {
		case <-b.ctx.Done():
			for ch := range subs {
				close(ch)
			}
			return
		case msg := <-b.subChan:
			subs[msg.data] = struct{}{}
			if msg.done != nil {
				msg.done()
			}
		case msg := <-b.unsubChan:
			var fullCh chan T
			for s := range subs {
				var sc <-chan T
				sc = s
				if sc == msg.data {
					fullCh = s
					break
				}
			}
			if fullCh != nil {
				delete(subs, fullCh)
				close(fullCh)
			}
			if msg.done != nil {
				msg.done()
			}
		case msg := <-b.publishChan:
			var wg *sync.WaitGroup
			usingWaitGroup := msg.done != nil
			if usingWaitGroup {
				wg = &sync.WaitGroup{}
				wg.Add(len(subs))
			}
			for ch := range subs {
				go func(ch chan T, msg T) {
					if usingWaitGroup {
						defer wg.Done()
					}
					select {
					case <-b.ctx.Done():
						return
					case ch <- msg:
					}
				}(ch, msg.data)
			}
			if usingWaitGroup {
				wg.Wait()
			}
			if msg.done != nil {
				msg.done()
			}
		}
	}
}

func sendAndWaitForDone[T any](ctx context.Context, data T, ch chan chanMsg[T]) bool {
	doneCh := make(chan struct{})
	select {
	case <-ctx.Done():
		return false
	case ch <- chanMsg[T]{data: data, done: func() { doneCh<-struct{}{} }}:
	}
	select {
	case <-ctx.Done():
		return false
	case <-doneCh:
		return true
	}
}

func sendNoWait[T any](ctx context.Context, data T, ch chan chanMsg[T]) {
	go func() {
		select {
		case <-ctx.Done():
			return
		case ch <- chanMsg[T]{data: data, done: nil}:
		}
	}()
}

func (b *broker[T]) Subscribe() <-chan T {
	ch := make(chan T)
	sent := sendAndWaitForDone(b.ctx, ch, b.subChan)
	if !sent {
		return nil
	}
	return ch
}

func (b *broker[T]) Unsubscribe(ch <-chan T) {
	_ = sendAndWaitForDone(b.ctx, ch, b.unsubChan)
}

func NoWait(pm *PublishModifier) {
	pm.nowait = true
}

func (b *broker[T]) Publish(msg T, mod ...func(*PublishModifier)) {
	cfg := modifiers.MakeMods(mod)
	if cfg.nowait {
		sendNoWait(b.ctx, msg, b.publishChan)
	} else {
		_ = sendAndWaitForDone(b.ctx, msg, b.publishChan)
	}
}
