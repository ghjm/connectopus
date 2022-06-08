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

type Router[T ~string] interface {

	// UpdateNode updates the connection information for a node.  The zero value of T is considered an invalid node.
	UpdateNode(T, map[T]float32)

	// RemoveNode removes the connection information for a node
	RemoveNode(T)

	// NextHop returns the next hop according to the routing policy.  Note that the policy may be out of date with the
	// latest connection information, if new information has been received within the duration specified by updateWait.
	// If there is no route to the node, the zero value is returned.
	NextHop(T) T

	// SubscribeUpdates returns a channel which will be sent a routing policy whenever it is updated
	SubscribeUpdates() <-chan map[T]T

	// UnsubscribeUpdates unsubscribes a previously subscribed updates channel
	UnsubscribeUpdates(ch <-chan map[T]T)

	// Nodes returns a copy of the nodes and connections known to the router
	Nodes() map[T]map[T]float32
}

// Implements Router[T]
type router[T ~string] struct {
	ctx           context.Context
	myNode        T
	nodes         syncro.Map[T, map[T]float32]
	policy        syncro.Map[T, T]
	updateWait    time.Duration
	tr            timerunner.TimeRunner
	zeroNode      T
	updatesBroker broker.Broker[map[T]T]
}

// New returns a new router.  UpdateWait specifies the maximum time after an UpdateNode that a recalculation should occur.
func New[T ~string](ctx context.Context, myNode T, updateWait time.Duration) Router[T] {
	r := &router[T]{
		ctx:           ctx,
		myNode:        myNode,
		nodes:         syncro.Map[T, map[T]float32]{},
		policy:        syncro.Map[T, T]{},
		updateWait:    updateWait,
		zeroNode:      *new(T),
		updatesBroker: broker.New[map[T]T](ctx),
	}
	r.tr = timerunner.New(ctx, r.recalculate)
	return r
}

func (r *router[T]) UpdateNode(node T, conns map[T]float32) {
	if node == r.zeroNode {
		panic("router node cannot be empty string")
	}
	r.nodes.WorkWith(func(_n *map[T]map[T]float32) {
		nodes := *_n
		if reflect.DeepEqual(nodes[node], conns) {
			return
		}
		nodes[node] = conns
		r.tr.RunWithin(r.updateWait)
	})
}

func (r *router[T]) RemoveNode(node T) {
	r.nodes.WorkWith(func(_n *map[T]map[T]float32) {
		nodes := *_n
		_, ok := nodes[node]
		if ok {
			delete(nodes, node)
			r.tr.RunWithin(r.updateWait)
		}
	})
}

// recalculate uses Dijkstra's algorithm to produce a routing policy
func (r *router[T]) recalculate() {
	r.nodes.WorkWithReadOnly(func(nodes map[T]map[T]float32) {
		Q := priorityQueue.New()
		Q.Insert(r.myNode, 0.0)
		cost := make(map[T]float32)
		prev := make(map[T]T)
		allNodes := make(map[T]struct{})
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
			prev[node] = r.zeroNode
			Q.Insert(node, float64(cost[node]))
		}
		for Q.Len() > 0 {
			nodeIf, _ := Q.Pop()
			node := nodeIf.(T)
			for neighbor, edgeCost := range nodes[node] {
				pathCost := cost[node] + edgeCost
				if pathCost < cost[neighbor] {
					cost[neighbor] = pathCost
					prev[neighbor] = node
					Q.Insert(neighbor, float64(pathCost))
				}
			}
		}
		newPolicy := make(map[T]T)
		for dest := range allNodes {
			p := dest
			for {
				if prev[p] == r.myNode {
					newPolicy[dest] = p
					break
				} else if prev[p] == r.zeroNode {
					break
				}
				p = prev[p]
			}
		}
		changed := false
		r.policy.WorkWith(func(policy *map[T]T) {
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

func (r *router[T]) NextHop(dest T) T {
	hop, ok := r.policy.Get(dest)
	if !ok {
		return r.zeroNode
	}
	return hop
}

func (r *router[T]) SubscribeUpdates() <-chan map[T]T {
	return r.updatesBroker.Subscribe()
}

func (r *router[T]) UnsubscribeUpdates(ch <-chan map[T]T) {
	r.updatesBroker.Unsubscribe(ch)
}

func (r *router[T]) Nodes() map[T]map[T]float32 {
	nodesCopy := make(map[T]map[T]float32)
	r.nodes.WorkWithReadOnly(func(nodes map[T]map[T]float32) {
		for k, v := range nodes {
			nodesCopy[k] = make(map[T]float32)
			for k2, v2 := range v {
				nodesCopy[k][k2] = v2
			}
		}
	})
	return nodesCopy
}
