package localui

import (
	"context"
	"fmt"
)

type Resolver struct {
}

func (r *Resolver) Query() QueryResolver {
	return r
}

func (r *Resolver) SSHKeys(_ context.Context) ([]*SSHKeyResult, error) {
	return nil, fmt.Errorf("not implemented")
}
