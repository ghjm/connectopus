package expector

import (
	"context"
	"github.com/ghjm/connectopus/pkg/x/syncro"
)

type Expector[T any] struct {
	ctx     context.Context
	expect  chan T
	produce chan T
	clear   chan struct{}
}

func NewExpector[T comparable](ctx context.Context) *Expector[T] {
	e := &Expector[T]{
		ctx:     ctx,
		expect:  make(chan T),
		produce: make(chan T),
		clear:   make(chan struct{}),
	}
	go func() {
		expectations := syncro.Map[T, struct{}]{}
		for {
			select {
			case <-ctx.Done():
				close(e.clear)
				return
			case item := <-e.expect:
				expectations.Set(item, struct{}{})
			case item := <-e.produce:
				cleared := false
				expectations.WorkWith(func(_exp *map[T]struct{}) {
					exp := *_exp
					delete(exp, item)
					cleared = len(exp) == 0
				})
				if cleared {
					go func() {
						select {
						case <-ctx.Done():
							return
						case e.clear <- struct{}{}:
						}
					}()
				}
			}
		}
	}()
	return e
}

func (e *Expector[T]) Expect(item T) {
	select {
	case <-e.ctx.Done():
	case e.expect <- item:
	}
}

func (e *Expector[T]) Produce(item T) {
	select {
	case <-e.ctx.Done():
	case e.produce <- item:
	}
}

func (e *Expector[T]) WaitClear() {
	select {
	case <-e.ctx.Done():
	case <-e.clear:
	}
}
