package localui

import (
	"context"
	"fmt"
	"github.com/google/uuid"
)

type Resolver struct {
	KeepaliveFunc       func()
	PreferredSocketFunc func(string)
}

func (r *Resolver) Query() QueryResolver {
	return r
}

func (r *Resolver) Mutation() MutationResolver {
	return r
}

func (r *Resolver) SSHKeys(_ context.Context) ([]*SSHKeyResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *Resolver) Keepalive(_ context.Context) (*KeepaliveResult, error) {
	r.KeepaliveFunc()
	return &KeepaliveResult{MutationID: uuid.New().String()}, nil
}

func (r *Resolver) Authenticate(_ context.Context, _ SSHKeyInput) (*AuthenticateResult, error) {
	return nil, fmt.Errorf("not implemented")
}
