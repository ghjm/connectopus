package syncro

import (
	"go.uber.org/goleak"
	"testing"
)

func TestSyncromap(t *testing.T) {
	defer goleak.VerifyNone(t)

	s := Map[string, int]{}
	s.Set("foo", 1)
	v, ok := s.Get("foo")
	if v != 1 || ok != true {
		t.Fail()
	}
	s.WorkWith(func(m *map[string]int) {
		v, ok = (*m)["foo"]
		if v != 1 || ok != true {
			t.Fail()
		}
		(*m)["foo"] = v + 1
		(*m)["bar"] = 123
	})
	v, ok = s.Get("foo")
	if v != 2 || ok != true {
		t.Fail()
	}
	v, ok = s.Get("bar")
	if v != 123 || ok != true {
		t.Fail()
	}
}

func TestMapFromGoMap(t *testing.T) {
	defer goleak.VerifyNone(t)
	m := NewMap[string, string](map[string]string{"hello": "goodbye", "i am": "the walrus"})
	v, ok := m.Get("hello")
	if !ok || v != "goodbye" {
		t.Fail()
	}
}
