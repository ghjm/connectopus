package router

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/pkg/utils/syncrovar"
	"github.com/ghjm/connectopus/pkg/utils/timerunner"
	priorityQueue "github.com/jupp0r/go-priority-queue"
	"math"
	"time"
)

type Router interface {

	// UpdateNode updates the connection information for a node.  Panics if the node is the empty string.
	UpdateNode(string, map[string]float32)

	// RemoveNode removes the connection information for a node
	RemoveNode(string)

	// NextHop returns the next hop according to the routing policy.  Note that the policy may be out of date with the
	// latest connection information, if new information has been received within the duration specified by updateWait.
	// If there is no route to the node, the empty string is returned.
	NextHop(string) string
}

type router struct {
	myNode     string
	nodes      map[string]map[string]float32
	policy     syncrovar.SyncroVar[map[string]string]
	updateChan chan nodeUpdate
	removeChan chan string
	updateWait time.Duration
	tr         timerunner.TimeRunner
}

type nodeUpdate struct {
	node  string
	conns map[string]float32
}

// New returns a new router.  UpdateWait specifies the maximum time after an UpdateNode that a recalculation should occur.
func New(ctx context.Context, myNode string, updateWait time.Duration) Router {
	r := &router{
		myNode:     myNode,
		nodes:      make(map[string]map[string]float32),
		updateChan: make(chan nodeUpdate),
		removeChan: make(chan string),
		updateWait: updateWait,
	}
	// updateNode and removeNode are run within the timerunner goroutine, to avoid the need for locks/mutexes
	r.tr = timerunner.NewTimeRunner(ctx, r.recalculate,
		timerunner.EventChan(r.updateChan, r.updateNode),
		timerunner.EventChan(r.removeChan, r.removeNode),
	)
	return r
}

func (r *router) updateNode(u nodeUpdate) {
	r.nodes[u.node] = u.conns
	r.tr.RunWithin(r.updateWait)
}

func (r *router) UpdateNode(node string, conns map[string]float32) {
	if node == "" {
		panic("router node cannot be empty string")
	}
	r.updateChan <- nodeUpdate{
		node:  node,
		conns: conns,
	}
}

func (r *router) removeNode(node string) {
	delete(r.nodes, node)
	r.tr.RunWithin(r.updateWait)
}

func (r *router) RemoveNode(node string) {
	r.removeChan <- node
}

// recalculate uses Dijkstra's algorithm to produce a routing policy
func (r *router) recalculate() {
	Q := priorityQueue.New()
	Q.Insert(r.myNode, 0.0)
	cost := make(map[string]float32)
	prev := make(map[string]string)
	for node := range r.nodes {
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
		node := fmt.Sprintf("%v", nodeIf)
		for neighbor, edgeCost := range r.nodes[node] {
			pathCost := cost[node] + edgeCost
			if pathCost < cost[neighbor] {
				cost[neighbor] = pathCost
				prev[neighbor] = node
				Q.Insert(neighbor, float64(pathCost))
			}
		}
	}
	newPolicy := make(map[string]string)
	for dest := range r.nodes {
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
	r.policy.Set(newPolicy)
}

func (r *router) NextHop(dest string) string {
	hop := ""
	r.policy.WorkWithReadOnly(func(policy *map[string]string) {
		if policy == nil {
			hop = ""
		} else {
			hop, _ = (*policy)[dest]
		}
	})
	return hop
}
