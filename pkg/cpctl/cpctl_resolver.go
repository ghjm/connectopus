package cpctl

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/pkg/links/netns"
	"github.com/ghjm/connectopus/pkg/netopus"
	"time"
)

type Resolver struct {
	NsReg             *netns.Registry
	NetopusStatusFunc func() *netopus.Status
}

func (r *Resolver) Query() QueryResolver {
	return r
}

func (r *Resolver) Netns(ctx context.Context, filter *NetnsFilter) ([]*NetnsResult, error) {
	var nspid []netns.NamePID
	if r.NsReg != nil {
		if filter == nil || filter.Name == nil {
			nspid = r.NsReg.GetAll()
		} else {
			nsp, err := r.NsReg.Get(*filter.Name)
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

func (r *Resolver) Status(ctx context.Context) (*Status, error) {
	if r.NetopusStatusFunc == nil {
		return nil, fmt.Errorf("unable to retrieve status")
	}
	ns := r.NetopusStatusFunc()
	stat := &Status{
		Name: ns.Name,
		Addr: ns.Addr.String(),
	}
	stat.NodeNames = make([]*StatusNodeName, 0)
	for nodeAddr, nodeName := range ns.NodeNames {
		stat.NodeNames = append(stat.NodeNames, &StatusNodeName{
			Addr: nodeAddr,
			Name: nodeName,
		})
	}
	stat.RouterNodes = make([]*StatusRouterNode, 0)
	for routerNode, routerRoutes := range ns.RouterNodes {
		rn := &StatusRouterNode{
			Node: routerNode,
		}
		rn.Peers = make([]*StatusRouterNodePeer, 0)
		for node, cost := range routerRoutes {
			rn.Peers = append(rn.Peers, &StatusRouterNodePeer{
				Node: node,
				Cost: float64(cost),
			})
		}
		stat.RouterNodes = append(stat.RouterNodes, rn)
	}
	stat.Sessions = make([]*StatusSession, 0)
	for sessAddr, sessStatus := range ns.Sessions {
		stat.Sessions = append(stat.Sessions, &StatusSession{
			Addr:      sessAddr,
			Connected: sessStatus.Connected,
			ConnStart: sessStatus.ConnStart.Format(time.RFC3339),
		})
	}
	return stat, nil
}

func (r *Resolver) Mutation() MutationResolver {
	return r
}

func (r *Resolver) Dummy(ctx context.Context) (*DummyResult, error) {
	return &DummyResult{}, nil
}
