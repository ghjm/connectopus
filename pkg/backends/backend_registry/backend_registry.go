package backend_registry

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/pkg/backends"
	"github.com/ghjm/connectopus/pkg/backends/backend_dtls"
	"github.com/ghjm/connectopus/pkg/config"
	"github.com/ghjm/connectopus/pkg/x/syncro"
)

type BackendRunFunc func(context.Context, backends.ProtocolRunner, float32, config.Params) error

var backendMap = syncro.NewMap[string, BackendRunFunc](map[string]BackendRunFunc{
	"dtls-dialer":   backend_dtls.RunDialerFromConfig,
	"dtls-listener": backend_dtls.RunListenerFromConfig,
})

var ErrUnknownBackend = fmt.Errorf("unknown backend")

func RunBackend(ctx context.Context, pr backends.ProtocolRunner, name string, cost float32, params config.Params) error {
	runner, ok := backendMap.Get(name)
	if !ok {
		return ErrUnknownBackend
	}
	return runner(ctx, pr, cost, params)
}
