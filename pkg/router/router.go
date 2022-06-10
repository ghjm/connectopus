package router

import (
	"context"
	"github.com/ghjm/connectopus/pkg/x/broker"
	"github.com/ghjm/connectopus/pkg/x/syncro"
	"github.com/ghjm/connectopus/pkg/x/timerunner"
	priorityQueue "github.com/jupp0r/go-priority-queue"
	"math"
	"reflect"
	"time"
)

type Router interface {

	// UpdateNode updates the connection information for a node.
	UpdateNode(string, map[string]float32)

	// RemoveNode removes the connection information for a node
	RemoveNode(string)

	// NextHop returns the next hop according to the routing policy.  Note that the policy may be out of date with the
	// latest connection information, if new information has been received within the duration specified by updateWait.
	// If there is no route to the node, the zero value is returned.
	NextHop(string) string

	// SubscribeUpdates returns a channel which will be sent a routing policy whenever it is updated
	SubscribeUpdates() <-chan map[string]string

	// UnsubscribeUpdates unsubscribes a previously subscribed updates channel
	UnsubscribeUpdates(ch <-chan map[string]string)

	// Nodes returns a copy of the nodes and connections known to the router
	Nodes() map[string]map[string]float32
}

// Implements Router
type router struct {
	ctx           context.Context
	myNode        string
	nodes         syncro.Map[string, map[string]float32]
	policy        syncro.Map[string, string]
	updateWait    time.Duration
	tr            timerunner.TimeRunner
	updatesBroker broker.Broker[map[string]string]
}

// New returns a new router.  UpdateWait specifies the maximum time after an UpdateNode that a recalculation should occur.
func New(ctx context.Context, myNode string, updateWait time.Duration) Router {
	r := &router{
		ctx:           ctx,
		myNode:        myNode,
		nodes:         syncro.Map[string, map[string]float32]{},
		policy:        syncro.Map[string, string]{},
		updateWait:    updateWait,
		updatesBroker: broker.New[map[string]string](ctx),
	}
	r.tr = timerunner.New(ctx, r.recalculate)
	return r
}

func (r *router) UpdateNode(node string, conns map[string]float32) {
	if node == "" {
		panic("router node cannot be empty string")
	}
	r.nodes.WorkWith(func(_n *map[string]map[string]float32) {
		nodes := *_n
		if reflect.DeepEqual(nodes[node], conns) {
			return
		}
		nodes[node] = conns
		r.tr.RunWithin(r.updateWait)
	})
}

func (r *router) RemoveNode(node string) {
	r.nodes.WorkWith(func(_n *map[string]map[string]float32) {
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
	r.nodes.WorkWithReadOnly(func(nodes map[string]map[string]float32) {
		Q := priorityQueue.New()
		Q.Insert(r.myNode, 0.0)
		cost := make(map[string]float32)
		prev := make(map[string]string)
		allNodes := make(map[string]struct{})
		for node := range nodes {
			allNodes[node] = struct{}{}
			for conn := range nodes[node] {
				allNodes[conn] = struct{}{}
			}
		}
		for node := range allNodes {
			if node == r.myNode {
				cost[node] = 0.0
			} else {
				cost[node] = math.MaxFloat32
			}
			prev[node] = ""
			Q.Insert(node, float64(cost[node]))
		}
		for Q.Len() > 0 {
			nodeIf, _ := Q.Pop()
			node := nodeIf.(string)
			for neighbor, edgeCost := range nodes[node] {
				pathCost := cost[node] + edgeCost
				if pathCost < cost[neighbor] {
					cost[neighbor] = pathCost
					prev[neighbor] = node
					Q.Insert(neighbor, float64(pathCost))
				}
			}
		}
		newPolicy := make(map[string]string)
		for dest := range allNodes {
			p := dest
			for {
				if prev[p] == r.myNode {
					newPolicy[dest] = p
					break
				} else if prev[p] == "" {
					break
				}
				p = prev[p]
			}
		}
		changed := false
		r.policy.WorkWith(func(policy *map[string]string) {
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
		if changed {
			r.updatesBroker.Publish(newPolicy)
		}
	})
}

func (r *router) NextHop(dest string) string {
	hop, ok := r.policy.Get(dest)
	if !ok {
		return ""
	}
	return hop
}

func (r *router) SubscribeUpdates() <-chan map[string]string {
	return r.updatesBroker.Subscribe()
}

func (r *router) UnsubscribeUpdates(ch <-chan map[string]string) {
	r.updatesBroker.Unsubscribe(ch)
}

func (r *router) Nodes() map[string]map[string]float32 {
	nodesCopy := make(map[string]map[string]float32)
	r.nodes.WorkWithReadOnly(func(nodes map[string]map[string]float32) {
		for k, v := range nodes {
			nodesCopy[k] = make(map[string]float32)
			for k2, v2 := range v {
				nodesCopy[k][k2] = v2
			}
		}
	})
	return nodesCopy
}
