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

// AddSend adds a channel send to the selector.  The provided function f, if not nil, will be called if this
// case is selected.
func AddSend[T any](s *Selector, ch chan<- T, v T, f func()) {
	s.cases = append(s.cases, selCase{
		sc: reflect.SelectCase{
			Dir:  reflect.SelectSend,
			Chan: reflect.ValueOf(ch),
			Send: reflect.ValueOf(v),
		},
		f: func(reflect.Value, bool) {
			if f != nil {
				f()
			}
		},
	})
}

// AddRecv adds a channel receive to the selector.  The provided function f, if not nil, will be called if this
// case is selected, with the received value and "ok" value as parameters.
func AddRecv[T any](s *Selector, ch <-chan T, f func(T, bool)) {
	s.cases = append(s.cases, selCase{
		sc: reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(ch),
		},
		f: func(v reflect.Value, ok bool) {
			if f != nil {
				f(v.Interface().(T), ok)
			}
		},
	})
}

// AddRecvDiscard adds a channel receive to the selector, which will discard the received value.  If the function f
// is not nil, it will be called when this item is selected, but it will not pass a value.
func AddRecvDiscard[T any](s *Selector, ch <-chan T, f func()) {
	AddRecv[T](s, ch, func(T, bool) {
		if f != nil {
			f()
		}
	})
}

// AddDefault adds a default case to the selector
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

// Select runs the select, using whatever cases have been added
func (s *Selector) Select() {
	cases := make([]reflect.SelectCase, len(s.cases))
	for i := range s.cases {
		cases[i] = s.cases[i].sc
	}
	chosen, recv, recvOK := reflect.Select(cases)
	s.cases[chosen].f(recv, recvOK)
}
