package router

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/pkg/proto"
	"github.com/ghjm/golib/pkg/broker"
	"github.com/ghjm/golib/pkg/syncro"
	"github.com/ghjm/golib/pkg/timerunner"
	priorityQueue "github.com/jupp0r/go-priority-queue"
	"math"
	"reflect"
	"time"
)

type Router interface {

	// UpdateNode updates the connection information for a node.
	UpdateNode(proto.IP, proto.RoutingConns)

	// RemoveNode removes the connection information for a node
	RemoveNode(proto.IP)

	// NextHop returns the next hop according to the routing policy.  Note that the policy may be out of date with the
	// latest connection information, if new information has been received within the duration specified by updateWait.
	NextHop(proto.IP) (proto.IP, error)

	// SubscribeUpdates returns a channel which will be sent a routing policy whenever it is updated
	SubscribeUpdates() <-chan proto.RoutingPolicy

	// UnsubscribeUpdates unsubscribes a previously subscribed updates channel
	UnsubscribeUpdates(ch <-chan proto.RoutingPolicy)

	// Nodes returns a copy of the nodes and connections known to the router
	Nodes() proto.RoutingNodes
}

// Implements Router
type router struct {
	ctx           context.Context
	myAddr        proto.IP
	nodes         syncro.Map[proto.IP, proto.RoutingConns]
	policy        syncro.Map[proto.Subnet, proto.IP]
	updateWait    time.Duration
	tr            timerunner.TimeRunner
	updatesBroker broker.Broker[proto.RoutingPolicy]
}

// New returns a new router.  UpdateWait specifies the maximum time after an UpdateNode that a recalculation should occur.
func New(ctx context.Context, myAddr proto.IP, updateWait time.Duration) Router {
	r := &router{
		ctx:           ctx,
		myAddr:        myAddr,
		nodes:         syncro.Map[proto.IP, proto.RoutingConns]{},
		policy:        syncro.Map[proto.Subnet, proto.IP]{},
		updateWait:    updateWait,
		updatesBroker: broker.New[proto.RoutingPolicy](ctx),
	}
	r.tr = timerunner.New(ctx, r.recalculate)
	return r
}

func (r *router) UpdateNode(node proto.IP, conns proto.RoutingConns) {
	if node == "" {
		panic("router node cannot be empty string")
	}
	r.nodes.WorkWith(func(_n *map[proto.IP]proto.RoutingConns) {
		nodes := *_n
		if reflect.DeepEqual(nodes[node], conns) {
			return
		}
		nodes[node] = conns
		r.tr.RunWithin(r.updateWait)
	})
}

func (r *router) RemoveNode(node proto.IP) {
	r.nodes.WorkWith(func(_n *map[proto.IP]proto.RoutingConns) {
		nodes := *_n
		_, ok := nodes[node]
		if ok {
			delete(nodes, node)
			r.tr.RunWithin(r.updateWait)
		}
	})
}

// recalculate uses Dijkstra's algorithm to produce a routing policy
func (r *router) recalculate() {
	r.nodes.WorkWithReadOnly(func(nodes map[proto.IP]proto.RoutingConns) {
		interconnect := make(map[proto.IP]map[proto.IP]float32)
		subnets := make(map[proto.Subnet]map[proto.IP]struct{})
		for node := range nodes {
			interconnect[node] = make(map[proto.IP]float32)
			for peer := range nodes {
				for conn, cost := range nodes[node] {
					if conn.Contains(peer) {
						interconnect[node][peer] = cost
					}
				}
			}
			for conn := range nodes[node] {
				if conn.Prefix() == 128 {
					_, ok := nodes[conn.IP()]
					if ok {
						continue
					}
				}
				_, ok := subnets[conn]
				if !ok {
					subnets[conn] = make(map[proto.IP]struct{})
				}
				subnets[conn][node] = struct{}{}
			}
		}
		Q := priorityQueue.New()
		Q.Insert(r.myAddr, 0.0)
		cost := make(map[proto.IP]float32)
		prev := make(map[proto.IP]proto.IP)
		for node := range nodes {
			if node == r.myAddr {
				cost[node] = 0.0
			} else {
				cost[node] = math.MaxFloat32
			}
			prev[node] = ""
			Q.Insert(node, float64(cost[node]))
		}
		for Q.Len() > 0 {
			nodeIf, _ := Q.Pop()
			node := nodeIf.(proto.IP)
			for neighbor, edgeCost := range interconnect[node] {
				pathCost := cost[node] + edgeCost
				if pathCost < cost[neighbor] {
					cost[neighbor] = pathCost
					prev[neighbor] = node
					Q.Insert(neighbor, float64(pathCost))
				}
			}
		}
		newPolicy := make(proto.RoutingPolicy)
		for dest := range nodes {
			p := dest
			for {
				if prev[p] == r.myAddr {
					newPolicy[proto.NewHostOnlySubnet(dest)] = p
					break
				} else if prev[p] == "" {
					break
				}
				p = prev[p]
			}
		}
		for subnet, candidates := range subnets {
			var bestCandidate proto.IP
			lowestCost := float32(math.MaxFloat32)
			for candidate := range candidates {
				via := newPolicy[proto.NewHostOnlySubnet(candidate)]
				if cost[via] < lowestCost {
					lowestCost = cost[via]
					bestCandidate = via
				}
			}
			if bestCandidate != "" {
				newPolicy[subnet] = bestCandidate
			}
		}
		changed := false
		r.policy.WorkWith(func(policy *map[proto.Subnet]proto.IP) {
			if len(*policy) != len(newPolicy) {
				changed = true
			} else {
				for k, v := range *policy {
					nv, ok := newPolicy[k]
					if !ok || nv != v {
						changed = true
						break
					}
				}
			}
			if changed {
				*policy = newPolicy
			}
		})
		r.updatesBroker.Publish(newPolicy)
	})
}

var ErrNoRouteToHost = fmt.Errorf("no route to host")

func (r *router) NextHop(dest proto.IP) (proto.IP, error) {
	var hop proto.IP
	longestPrefix := -1
	r.policy.WorkWithReadOnly(func(p map[proto.Subnet]proto.IP) {
		for k, v := range p {
			if k.Contains(dest) {
				prefix := k.Prefix()
				if prefix > longestPrefix {
					hop = v
					longestPrefix = prefix
				}
			}
		}
	})
	if longestPrefix == -1 {
		return "", ErrNoRouteToHost
	}
	return hop, nil
}

func (r *router) SubscribeUpdates() <-chan proto.RoutingPolicy {
	return r.updatesBroker.Subscribe()
}

func (r *router) UnsubscribeUpdates(ch <-chan proto.RoutingPolicy) {
	r.updatesBroker.Unsubscribe(ch)
}

func (r *router) Nodes() proto.RoutingNodes {
	nodesCopy := make(proto.RoutingNodes)
	r.nodes.WorkWithReadOnly(func(nodes map[proto.IP]proto.RoutingConns) {
		for k, v := range nodes {
			nodesCopy[k] = make(proto.RoutingConns)
			for k2, v2 := range v {
				nodesCopy[k][k2] = v2
			}
		}
	})
	return nodesCopy
}
