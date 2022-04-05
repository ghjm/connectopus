package syncromap

import (
	"go.uber.org/goleak"
	"testing"
)

func TestSyncromap(t *testing.T) {
	defer goleak.VerifyNone(t)

	s := NewMap[string, int]()
	s.Set("foo", 1)
	v, ok := s.Get("foo")
	if v != 1 || ok != true {
		t.Fail()
	}
	tr := s.BeginTransaction()
	v, ok = tr.Get("foo")
	if v != 1 || ok != true {
		t.Fail()
	}
	tr.Set("foo", v+1)
	tr.EndTransaction()
	v, ok = s.Get("foo")
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
