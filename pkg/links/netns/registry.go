package netns

import (
	"fmt"
	"github.com/ghjm/golib/pkg/syncro"
)

type Registry struct {
	m syncro.Map[string, int]
}

type NamePID struct {
	Name string
	PID  int
}

func (ns *Registry) Add(name string, pid int) {
	ns.m.Set(name, pid)
}

func (ns *Registry) Del(name string) {
	ns.m.Delete(name)
}

func (ns *Registry) GetAll() []NamePID {
	var result []NamePID
	ns.m.WorkWithReadOnly(func(m map[string]int) {
		for k, v := range m {
			result = append(result, NamePID{
				Name: k,
				PID:  v,
			})
		}
	})
	return result
}

func (ns *Registry) Get(name string) (int, error) {
	pid, ok := ns.m.Get(name)
	if !ok {
		return 0, fmt.Errorf("unknown namespace")
	}
	return pid, nil
}
