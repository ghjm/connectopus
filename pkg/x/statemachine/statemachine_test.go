package statemachine

import (
	"go.uber.org/goleak"
	"testing"
)

func TestFSM(t *testing.T) {
	defer goleak.VerifyNone(t)

	m1 := New[string, string]()
	err := m1.Initialize("foo", StateMap[string, string]{
		"bar": func(e string) string { return "bar" },
	})
	if err == nil {
		t.Fatal("did not catch invalid initial state")
	}

	m2 := New[string, int]()
	err = m2.Initialize("foo", StateMap[string, int]{
		"foo": func(e int) string { return "bar" },
		"bar": func(e int) string { return "bad" },
		"bad": func(e int) string {
			if e == 1 {
				return "evil"
			}
			return "foo"
		},
	})
	if err != nil {
		t.Fatal("did not initialize")
	}
	for i := 0; i < 5; i++ {
		err = m2.Event(0)
		if err != nil {
			t.Fatal("event failed")
		}
	}
	if m2.State() != "bad" {
		t.Fatal("wrong state arrived at")
	}
	err = m2.Event(1)
	if err == nil {
		t.Fatal("did not catch invalid state")
	}
}
