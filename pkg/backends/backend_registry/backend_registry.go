package backend_registry

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/pkg/backends"
	"github.com/ghjm/connectopus/pkg/backends/backend_dtls"
	"github.com/ghjm/connectopus/pkg/backends/backend_tcp"
	"github.com/ghjm/connectopus/pkg/config"
	"github.com/ghjm/golib/pkg/syncro"
)

type BackendRunFunc func(context.Context, backends.ProtocolRunner, float32, config.Params) error

type BackendSpec struct {
	RunFunc BackendRunFunc
	MTU     uint16
}

var backendMap syncro.Map[string, BackendSpec]

// LookupBackend returns a BackendSpec, or nil if the spec doesn't exist.
func LookupBackend(name string) *BackendSpec {
	spec, ok := backendMap.Get(name)
	if !ok {
		return nil
	}
	return &spec
}

// RegisterBackend binds a name to a BackendSpec.
func RegisterBackend(name string, spec BackendSpec) {
	backendMap.Set(name, spec)
}

var ErrUnknownBackend = fmt.Errorf("unknown backend")

// RunBackend runs a ProtocolRunner over a backend.
func RunBackend(ctx context.Context, pr backends.ProtocolRunner, name string, cost float32, params config.Params) error {
	spec := LookupBackend(name)
	if spec == nil {
		return ErrUnknownBackend
	}
	return spec.RunFunc(ctx, pr, cost, params)
}

func init() {
	RegisterBackend("dtls-dialer", BackendSpec{
		RunFunc: backend_dtls.RunDialerFromConfig,
		MTU:     backend_dtls.MTU,
	})
	RegisterBackend("dtls-listener", BackendSpec{
		RunFunc: backend_dtls.RunListenerFromConfig,
		MTU:     backend_dtls.MTU,
	})
	RegisterBackend("tcp-dialer", BackendSpec{
		RunFunc: backend_tcp.RunDialerFromConfig,
		MTU:     backend_tcp.MTU,
	})
	RegisterBackend("tcp-listener", BackendSpec{
		RunFunc: backend_tcp.RunListenerFromConfig,
		MTU:     backend_tcp.MTU,
	})
}
