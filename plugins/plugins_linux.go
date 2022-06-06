//go:build linux

package plugins

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/pkg/config"
	"github.com/ghjm/connectopus/pkg/netstack"
	"plugin"
)

func RunPlugin(ctx context.Context, filename string, stack netstack.UserStack, params config.Params) error {
	plug, err := plugin.Open(filename)
	if err != nil {
		return err
	}
	symMain, err := plug.Lookup("PluginMain")
	if err != nil {
		return err
	}
	pluginMain, ok := symMain.(func(context.Context, netstack.UserStack, config.Params))
	if !ok {
		return fmt.Errorf("plugin main function is of wrong type")
	}
	go pluginMain(ctx, stack, params)
	return nil
}
