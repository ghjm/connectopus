package cpctl

import (
	"context"
	"github.com/ghjm/connectopus/pkg/netopus/link/netns"
)

type Resolver struct {
	NsReg *netns.Registry
}

func (r *Resolver) Query() QueryResolver {
	return r
}

func (r *Resolver) Netns(ctx context.Context, filter *NetnsFilter) ([]*NetnsResult, error) {
	var nspid []netns.NamePID
	if filter == nil || filter.Name == nil {
		nspid = r.NsReg.GetAll()
	} else {
		nsp, err := r.NsReg.Get(*filter.Name)
		if err == nil {
			nspid = []netns.NamePID{{Name: *filter.Name, PID: nsp}}
		}
	}
	nsr := make([]*NetnsResult, 0, len(nspid))
	for _, np := range nspid {
		nsr = append(nsr, &NetnsResult{np.Name, np.PID})
	}
	return nsr, nil
}

func (r *Resolver) Mutation() MutationResolver {
	return r
}

func (r *Resolver) Dummy(ctx context.Context) (*DummyResult, error) {
	return &DummyResult{}, nil
}
