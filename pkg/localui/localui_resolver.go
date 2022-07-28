package localui

import (
	"context"
	"github.com/google/uuid"
)

type Resolver struct {
	AvailableNodesFunc  func() ([]string, error)
	PreferredSocketFunc func(string) error
}

func (r *Resolver) Query() QueryResolver {
	return r
}

func (r *Resolver) Mutation() MutationResolver {
	return r
}

func (r *Resolver) AvailableNodes(_ context.Context) (*AvailableNodesResult, error) {
	nodes, err := r.AvailableNodesFunc()
	if err != nil {
		return nil, err
	}
	return &AvailableNodesResult{Node: nodes}, nil
}

func (r *Resolver) SelectNode(_ context.Context, node string) (*SelectNodeResult, error) {
	err := r.PreferredSocketFunc(node)
	if err != nil {
		return nil, err
	}
	return &SelectNodeResult{MutationID: uuid.New().String()}, nil
}
