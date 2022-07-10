package cpctl

import (
	"context"
	"github.com/ghjm/connectopus/pkg/config"
	"github.com/ghjm/connectopus/pkg/links/netns"
	"github.com/ghjm/connectopus/pkg/proto"
	"github.com/google/uuid"
	"golang.org/x/exp/slices"
	"gopkg.in/yaml.v3"
	"time"
)

type Resolver struct {
	GetConfig           func() *config.Config
	GetNetopus          func() proto.Netopus
	GetNsReg            func() *netns.Registry
	GetReconcilerStatus func() error
	UpdateNodeConfig    func([]byte) error
}

func (r *Resolver) Query() QueryResolver {
	return r
}

func (r *Resolver) Netns(_ context.Context, filter *NetnsFilter) ([]*NetnsResult, error) {
	var nspid []netns.NamePID
	nsReg := r.GetNsReg()
	if nsReg != nil {
		if filter == nil || filter.Name == nil {
			nspid = nsReg.GetAll()
		} else {
			nsp, err := nsReg.Get(*filter.Name)
			if err == nil {
				nspid = []netns.NamePID{{Name: *filter.Name, PID: nsp}}
			}
		}
	}
	nsr := make([]*NetnsResult, 0, len(nspid))
	for _, np := range nspid {
		nsr = append(nsr, &NetnsResult{np.Name, np.PID})
	}
	return nsr, nil
}

func (r *Resolver) Status(_ context.Context) (*Status, error) {
	c := r.GetConfig()
	ns := r.GetNetopus().Status()
	stat := &Status{
		Name: ns.Name,
		Addr: ns.Addr.String(),
		Global: &StatusGlobal{
			Domain: c.Global.Domain,
			Subnet: c.Global.Subnet.String(),
		},
	}
	for _, key := range c.Global.AuthorizedKeys {
		stat.Global.AuthorizedKeys = append(stat.Global.AuthorizedKeys, key.String())
	}
	for routerNode, routerRoutes := range ns.RouterNodes {
		rn := &StatusNode{
			Addr:  routerNode,
			Name:  ns.AddrToName[routerNode],
			Conns: nil,
		}
		rn.Conns = make([]*StatusNodeConn, 0)
		for node, cost := range routerRoutes {
			rn.Conns = append(rn.Conns, &StatusNodeConn{
				Subnet: node,
				Cost:   float64(cost),
			})
		}
		slices.SortFunc(rn.Conns, func(a, b *StatusNodeConn) bool {
			return a.Subnet < b.Subnet
		})
		stat.Nodes = append(stat.Nodes, rn)
	}
	slices.SortFunc(stat.Nodes, func(a, b *StatusNode) bool {
		return a.Addr < b.Addr
	})
	for sessAddr, sessStatus := range ns.Sessions {
		stat.Sessions = append(stat.Sessions, &StatusSession{
			Addr:      sessAddr,
			Connected: sessStatus.Connected,
			ConnStart: sessStatus.ConnStart.Format(time.RFC3339),
		})
	}
	slices.SortFunc(stat.Sessions, func(a, b *StatusSession) bool {
		return a.Addr < b.Addr
	})
	return stat, nil
}

func (r *Resolver) Mutation() MutationResolver {
	return r
}

func (r *Resolver) Config(_ context.Context) (*ConfigResult, error) {
	cfg := r.GetConfig()
	yamlCfg, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	return &ConfigResult{
		Yaml: string(yamlCfg),
	}, nil
}

func (r *Resolver) UpdateConfig(_ context.Context, config ConfigUpdateInput) (*ConfigUpdateResult, error) {
	err := r.UpdateNodeConfig([]byte(config.Yaml))
	return &ConfigUpdateResult{
		MutationID: uuid.NewString(),
	}, err
}
