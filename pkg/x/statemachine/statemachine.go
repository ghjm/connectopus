package statemachine

import (
	"fmt"
)

type TransitionFunc[S comparable, E any] func(E) S
type StateMap[S comparable, E any] map[S]TransitionFunc[S, E]

type FSM[S comparable, E any] interface {
	Initialize(S, StateMap[S, E]) error
	Event(E) error
	State() S
}

type fsm[S comparable, E any] struct {
	states   StateMap[S, E]
	curState S
}

var ErrInvalidState = fmt.Errorf("invalid state")

func New[S comparable, E any]() FSM[S, E] {
	m := &fsm[S, E]{
		states: make(StateMap[S, E]),
	}
	return m
}

func (m *fsm[S, E]) Initialize(initState S, states StateMap[S, E]) error {
	_, ok := states[initState]
	if !ok {
		return ErrInvalidState
	}
	m.curState = initState
	m.states = states
	return nil
}

func (m *fsm[S, E]) State() S {
	return m.curState
}

func (m *fsm[S, E]) Event(e E) error {
	tf, ok := m.states[m.curState]
	if !ok {
		return ErrInvalidState
	}
	s := tf(e)
	_, ok = m.states[s]
	if !ok {
		return ErrInvalidState
	}
	m.curState = s
	return nil
}
