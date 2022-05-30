package dynselect

import "reflect"

type selCase struct {
	sc reflect.SelectCase
	f  func(reflect.Value, bool)
}

// Selector is a generic wrapper around dynamic select.  It is instantiated and used as a zero value.
type Selector struct {
	cases []selCase
}

func AddSend[T any](s *Selector, ch chan<- T, v T, f func()) {
	s.cases = append(s.cases, selCase{
		sc: reflect.SelectCase{
			Dir:  reflect.SelectSend,
			Chan: reflect.ValueOf(ch),
			Send: reflect.ValueOf(v),
		},
		f: func(reflect.Value, bool) {
			f()
		},
	})
}

func AddRecv[T any](s *Selector, ch <-chan T, f func(T, bool)) {
	s.cases = append(s.cases, selCase{
		sc: reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(ch),
		},
		f: func(v reflect.Value, ok bool) {
			f(v.Interface().(T), ok)
		},
	})
}

// AddRecvDiscard adds a channel receiver that doesn't care about the returned value
func AddRecvDiscard[T any](s *Selector, ch <-chan T, f func()) {
	AddRecv(s, ch, func(T, bool) { f() })
}

func AddDefault(s *Selector, f func()) {
	s.cases = append(s.cases, selCase{
		sc: reflect.SelectCase{
			Dir: reflect.SelectDefault,
		},
		f: func(v reflect.Value, ok bool) {
			f()
		},
	})
}

func (s *Selector) Select() {
	cases := make([]reflect.SelectCase, len(s.cases))
	for i := range s.cases {
		cases[i] = s.cases[i].sc
	}
	chosen, recv, recvOK := reflect.Select(cases)
	s.cases[chosen].f(recv, recvOK)
}
