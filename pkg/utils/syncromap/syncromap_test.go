package syncromap

import (
	"go.uber.org/goleak"
	"testing"
)

func TestSyncromap(t *testing.T) {
	defer goleak.VerifyNone(t)

	s := NewMap[string, int]()
	s.Store("foo", 1)
	v, ok := s.Load("foo")
	if v != 1 || ok != true {
		t.Fail()
	}
	tr := s.BeginTransaction()
	v, ok = tr.Load("foo")
	if v != 1 || ok != true {
		t.Fail()
	}
	tr.Store("foo", v+1)
	tr.EndTransaction()
	v, ok = s.Load("foo")
	if v != 2 || ok != true {
		t.Fail()
	}
	func() {
		defer func() {
			r := recover()
			if r != ErrTransAfterEndTrans {
				t.Errorf("transaction access after EndTransaction did not panic")
			}
		}()
		tr.Delete("foo")
	}()
}
