package router

import (
	"context"
	"github.com/ghjm/connectopus/pkg/utils/timerunner"
	priorityQueue "github.com/jupp0r/go-priority-queue"
	"math"
	"sync"
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
}

type router[T ~string] struct {
	myNode     T
	nodes      map[T]map[T]float32
	policyLock *sync.RWMutex
	policy     map[T]T
	updateChan chan nodeUpdate[T]
	removeChan chan T
	updateWait time.Duration
	tr         timerunner.TimeRunner
	zeroNode   T
}

type nodeUpdate[T ~string] struct {
	node  T
	conns map[T]float32
}

// New returns a new router.  UpdateWait specifies the maximum time after an UpdateNode that a recalculation should occur.
func New[T ~string](ctx context.Context, myNode T, updateWait time.Duration) Router[T] {
	r := &router[T]{
		myNode:     myNode,
		nodes:      make(map[T]map[T]float32),
		policyLock: &sync.RWMutex{},
		policy:     make(map[T]T),
		updateChan: make(chan nodeUpdate[T]),
		removeChan: make(chan T),
		updateWait: updateWait,
		zeroNode:   *new(T),
	}
	// updateNode and removeNode are run within the timerunner goroutine, to avoid the need for locks/mutexes
	r.tr = timerunner.NewTimeRunner(ctx, r.recalculate,
		timerunner.EventChan(r.updateChan, r.updateNode),
		timerunner.EventChan(r.removeChan, r.removeNode),
	)
	return r
}

func (r *router[T]) updateNode(u nodeUpdate[T]) {
	r.nodes[u.node] = u.conns
	r.tr.RunWithin(r.updateWait)
}

func (r *router[T]) UpdateNode(node T, conns map[T]float32) {
	if node == r.zeroNode {
		panic("router node cannot be empty string")
	}
	r.updateChan <- nodeUpdate[T]{
		node:  node,
		conns: conns,
	}
}

func (r *router[T]) removeNode(node T) {
	delete(r.nodes, node)
	r.tr.RunWithin(r.updateWait)
}

func (r *router[NT]) RemoveNode(node NT) {
	r.removeChan <- node
}

// recalculate uses Dijkstra's algorithm to produce a routing policy
func (r *router[T]) recalculate() {
	Q := priorityQueue.New()
	Q.Insert(r.myNode, 0.0)
	cost := make(map[T]float32)
	prev := make(map[T]T)
	allNodes := make(map[T]struct{})
	for node := range r.nodes {
		allNodes[node] = struct{}{}
		for conn := range r.nodes[node] {
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
		for neighbor, edgeCost := range r.nodes[node] {
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
	r.policyLock.Lock()
	r.policy = newPolicy
	r.policyLock.Unlock()
}

func (r *router[T]) NextHop(dest T) T {
	r.policyLock.RLock()
	defer r.policyLock.RUnlock()
	hop, ok := r.policy[dest]
	if !ok {
		return r.zeroNode
	}
	return hop
}
