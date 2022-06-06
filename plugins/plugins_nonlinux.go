//go:build !linux

package plugins

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/pkg/config"
	"github.com/ghjm/connectopus/pkg/netstack"
)

func RunPlugin(ctx context.Context, filename string, stack netstack.UserStack, params config.Params) error {
	return fmt.Errorf("plugins are only supported on Linux")
}
