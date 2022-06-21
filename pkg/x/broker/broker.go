package broker

import (
	"context"
)

// broker code adapted from https://stackoverflow.com/questions/36417199/how-to-broadcast-message-using-channel
// which is licensed under Creative Commons CC BY-SA 4.0.

// Broker implements a fan-out system where multiple consumers can subscribe and receive published messages.
type Broker[T any] interface {
	// Publish dispatches a message to all subscribed receivers.  The function call does not block.
	Publish(T)
	// Subscribe returns a channel that will receive published messages.
	Subscribe() <-chan T
	// Unsubscribe stops sending messages and closes the channel.  The caller is
	// responsible for draining any remaining messages pending for the channel.
	Unsubscribe(<-chan T)
}

// broker implements Broker
type broker[T any] struct {
	ctx       context.Context
	publishCh chan T
	subCh     chan chan T
	unSubCh   chan (<-chan T)
}

// New starts a new broker.
func New[T any](ctx context.Context) Broker[T] {
	b := &broker[T]{
		ctx:       ctx,
		publishCh: make(chan T),
		subCh:     make(chan chan T),
		unSubCh:   make(chan (<-chan T)),
	}
	go b.run()
	return b
}

func (b *broker[T]) run() {
	subs := make(map[<-chan T]chan T)
	for {
		select {
		case <-b.ctx.Done():
			return
		case msgCh := <-b.subCh:
			subs[msgCh] = msgCh
		case msgCh := <-b.unSubCh:
			realCh := subs[msgCh]
			delete(subs, msgCh)
			if realCh != nil {
				close(realCh)
			}
		case msg := <-b.publishCh:
			for msgCh := range subs {
				func(msgCh chan T) {
					select {
					case <-b.ctx.Done():
					case msgCh <- msg:
					}
				}(subs[msgCh])
			}
		}
	}
}

func (b *broker[T]) Publish(msg T) {
	select {
	case <-b.ctx.Done():
	case b.publishCh <- msg:
	}
}

func (b *broker[T]) Subscribe() <-chan T {
	msgCh := make(chan T)
	select {
	case <-b.ctx.Done():
		return nil
	case b.subCh <- msgCh:
		return msgCh
	}
}

func (b *broker[T]) Unsubscribe(msgCh <-chan T) {
	go func() {
		for {
			select {
			case <-b.ctx.Done():
				return
			case _, ok := <-msgCh:
				if !ok {
					return
				}
			}
		}
	}()
	select {
	case <-b.ctx.Done():
	case b.unSubCh <- msgCh:
	}
}
