package connectopus

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/pkg/backends/backend_registry"
	"github.com/ghjm/connectopus/pkg/config"
	"github.com/ghjm/connectopus/pkg/cpctl"
	"github.com/ghjm/connectopus/pkg/links/netns"
	"github.com/ghjm/connectopus/pkg/links/tun"
	"github.com/ghjm/connectopus/pkg/netopus"
	"github.com/ghjm/connectopus/pkg/proto"
	"github.com/ghjm/connectopus/pkg/services"
	"github.com/ghjm/connectopus/pkg/x/reconciler"
	"github.com/ghjm/connectopus/pkg/x/ssh_jwt"
	"github.com/golang-jwt/jwt/v4"
	"net"
	"reflect"
)

func RunNode(ctx context.Context, cfgData []byte, identity string) (*reconciler.RunningItem, error) {
	cfg, err := config.ParseConfig(cfgData)
	if err != nil {
		return nil, fmt.Errorf("error parsing config: %w", err)
	}
	ri := reconciler.NewRunningItem(ctx, identity)
	nodeCfg := NodeCfg{
		Config:   cfg,
		identity: identity,
	}
	ri.Reconcile(nodeCfg, ri)
	return ri, ri.Status()
}

func UpdateNode(ri *reconciler.RunningItem, cfgData []byte) error {
	cfg, err := config.ParseConfig(cfgData)
	if err != nil {
		return fmt.Errorf("error parsing config: %w", err)
	}
	nodeCfg, ok := ri.Config().(NodeCfg)
	if !ok {
		return fmt.Errorf("running instance config is wrong type")
	}
	nodeCfg.Config = cfg
	ri.Reconcile(nodeCfg, ri)
	return ri.Status()
}

type NodeCfg struct {
	*config.Config
	identity string
}

func (nc NodeCfg) ParentEqual(item reconciler.ConfigItem) bool {
	ci, ok := item.(NodeCfg)
	if !ok {
		return false
	}
	if ci.identity != nc.identity {
		return false
	}
	if !reflect.DeepEqual(ci.Global, nc.Global) {
		return false
	}
	cn, cnOK := ci.Nodes[ci.identity]
	nn, nnOK := nc.Nodes[nc.identity]
	if !cnOK || !nnOK {
		return false
	}
	if cn.Address.String() != nn.Address.String() {
		return false
	}
	return true
}

type nodeInstance struct {
	cfg      *config.Config
	identity string
	node     *config.Node
	n        proto.Netopus
	nsreg    *netns.Registry
	ri       *reconciler.RunningItem
}

func (nc NodeCfg) Start(ctx context.Context, instance any) (any, error) {
	ri, ok := instance.(*reconciler.RunningItem)
	if !ok {
		return nil, fmt.Errorf("error retrieving instance: bad type")
	}
	node, ok := nc.Nodes[nc.identity]
	if !ok {
		return nil, fmt.Errorf("invalid identity for config")
	}
	inst := &nodeInstance{
		cfg:      nc.Config,
		identity: nc.identity,
		node:     &node,
		nsreg:    &netns.Registry{},
		ri:       ri,
	}
	var err error
	inst.n, err = netopus.New(ctx, node.Address, nc.identity, netopus.LeastMTU(node, 1500))
	if err != nil {
		return nil, err
	}
	return inst, nil
}

func (nc NodeCfg) Children() map[string]reconciler.ConfigItem {
	node, ok := nc.Config.Nodes[nc.identity]
	if !ok {
		return nil
	}
	children := make(map[string]reconciler.ConfigItem)
	children["cpctl"] = CpctlCfg(node.Cpctl)
	for k, v := range node.Backends {
		children[fmt.Sprintf("backend.%s", k)] = BackendCfg(v)
	}
	for k, v := range node.Namespaces {
		children[fmt.Sprintf("namespace.%s", k)] = NamespaceCfg(v)
	}
	for k, v := range node.Services {
		children[fmt.Sprintf("service.%s", k)] = ServiceCfg(v)
	}
	for k, v := range node.TunDevs {
		children[fmt.Sprintf("tundev.%s", k)] = TunDevCfg(v)
	}
	for k, v := range node.Namespaces {
		children[fmt.Sprintf("namespace.%s", k)] = NamespaceCfg(v)
	}
	return children
}

type CpctlCfg config.Cpctl

func (c CpctlCfg) ParentEqual(item reconciler.ConfigItem) bool {
	ci, ok := item.(CpctlCfg)
	if !ok {
		return false
	}
	return reflect.DeepEqual(ci, c)
}

func (c CpctlCfg) Start(ctx context.Context, instance any) (any, error) {
	inst, ok := instance.(*nodeInstance)
	if !ok {
		return nil, fmt.Errorf("error retrieving instance: bad type")
	}
	var sm jwt.SigningMethod
	{
		var err error
		sm, err = ssh_jwt.SetupSigningMethod("connectopus", nil)
		if err != nil {
			return nil, fmt.Errorf("error initializing JWT signing method: %w", err)
		}
	}
	csrv := cpctl.Server{
		Resolver: cpctl.Resolver{
			GetConfig: func() *config.Config {
				cfg := inst.ri.Config()
				ncfg, ncOK := cfg.(NodeCfg)
				if !ncOK {
					return nil
				}
				return ncfg.Config
			},
			GetNetopus:          func() proto.Netopus { return inst.n },
			GetNsReg:            func() *netns.Registry { return inst.nsreg },
			GetReconcilerStatus: func() error { return inst.ri.Status() },
			UpdateNodeConfig:    func(config []byte) error { return UpdateNode(inst.ri, config) },
		},
		SigningMethod: sm,
	}
	{
		li, err := inst.n.ListenOOB(ctx, cpctl.ProxyPortNo)
		if err != nil {
			return nil, fmt.Errorf("error initializing cpctl proxy listener: %w", err)
		}
		err = csrv.ServeHTTP(ctx, li)
		if err != nil {
			return nil, fmt.Errorf("error running cpctl proxy server: %w", err)
		}
	}
	if !c.NoSocket {
		socketFile, err := config.ExpandFilename(inst.identity, c.SocketFile)
		if err != nil {
			return nil, fmt.Errorf("error expanding socket filename: %w", err)
		}
		err = csrv.ServeUnix(ctx, socketFile)
		if err != nil {
			return nil, fmt.Errorf("error running socket server: %w", err)
		}
	}
	if c.Port != 0 {
		lc := net.ListenConfig{}
		li, err := lc.Listen(ctx, "tcp", fmt.Sprintf(":%d", c.Port))
		if err != nil {
			return nil, fmt.Errorf("error initializing cpctl web server: %w", err)
		}
		err = csrv.ServeHTTP(ctx, li)
		if err != nil {
			return nil, fmt.Errorf("error running cpctl web server: %w", err)
		}
	}
	return inst, nil
}

func (c CpctlCfg) Children() map[string]reconciler.ConfigItem {
	return nil
}

type BackendCfg config.Params

func (b BackendCfg) ParentEqual(item reconciler.ConfigItem) bool {
	ci, ok := item.(BackendCfg)
	if !ok {
		return false
	}
	return reflect.DeepEqual(ci, b)
}

func (b BackendCfg) Start(ctx context.Context, instance any) (any, error) {
	inst, ok := instance.(*nodeInstance)
	if !ok {
		return nil, fmt.Errorf("error retrieving instance: bad type")
	}
	p := config.Params(b)
	backendType := p.GetString("type", "")
	backendCost, err := p.GetFloat32("cost", 0)
	if err != nil {
		return nil, fmt.Errorf("backend configuration error: %w", err)
	}
	err = backend_registry.RunBackend(ctx, inst.n, backendType, defaultCost(backendCost), p)
	if err != nil {
		return nil, fmt.Errorf("error initializing backend: %w", err)
	}
	return inst, nil
}

func (b BackendCfg) Children() map[string]reconciler.ConfigItem {
	return nil
}

type ServiceCfg config.Service

func (s ServiceCfg) ParentEqual(item reconciler.ConfigItem) bool {
	ci, ok := item.(ServiceCfg)
	if !ok {
		return false
	}
	return reflect.DeepEqual(ci, s)
}

func (s ServiceCfg) Start(ctx context.Context, instance any) (any, error) {
	inst, ok := instance.(*nodeInstance)
	if !ok {
		return nil, fmt.Errorf("error retrieving instance: bad type")
	}
	_, err := services.RunService(ctx, inst.n, config.Service(s))
	if err != nil {
		return nil, fmt.Errorf("error initializing service: %w", err)
	}
	return inst, nil
}

func (s ServiceCfg) Children() map[string]reconciler.ConfigItem {
	return nil
}

type TunDevCfg config.TunDev

func (t TunDevCfg) ParentEqual(item reconciler.ConfigItem) bool {
	ci, ok := item.(TunDevCfg)
	if !ok {
		return false
	}
	return reflect.DeepEqual(ci, t)
}

func (t TunDevCfg) Start(ctx context.Context, instance any) (any, error) {
	inst, ok := instance.(*nodeInstance)
	if !ok {
		return nil, fmt.Errorf("error retrieving instance: bad type")
	}
	tunLink, err := tun.New(ctx, t.DeviceName, net.IP(t.Address), inst.cfg.Global.Subnet.AsIPNet(), inst.n.MTU())
	if err != nil {
		return nil, fmt.Errorf("error initializing tunnel: %w", err)
	}
	tunCh := tunLink.SubscribePackets()
	go func() {
		for {
			select {
			case <-ctx.Done():
				tunLink.UnsubscribePackets(tunCh)
				return
			case packet := <-tunCh:
				_ = inst.n.SendPacket(packet)
			}
		}
	}()
	inst.n.AddExternalRoute(t.Name, proto.NewHostOnlySubnet(t.Address), defaultCost(t.Cost), tunLink.SendPacket)
	return inst, nil
}

func (t TunDevCfg) Children() map[string]reconciler.ConfigItem {
	return nil
}

type NamespaceCfg config.Namespace

func (n NamespaceCfg) ParentEqual(item reconciler.ConfigItem) bool {
	ci, ok := item.(NamespaceCfg)
	if !ok {
		return false
	}
	return reflect.DeepEqual(ci, n)
}

func (n NamespaceCfg) Start(ctx context.Context, instance any) (any, error) {
	inst, ok := instance.(*nodeInstance)
	if !ok {
		return nil, fmt.Errorf("error retrieving instance: bad type")
	}
	ns, err := netns.New(ctx, net.IP(n.Address), netns.WithMTU(inst.n.MTU()))
	if err != nil {
		return nil, fmt.Errorf("error initializing namespace: %w", err)
	}
	nsCh := ns.SubscribePackets()
	go func() {
		for {
			select {
			case <-ctx.Done():
				ns.UnsubscribePackets(nsCh)
				return
			case packet := <-nsCh:
				_ = inst.n.SendPacket(packet)
			}
		}
	}()
	inst.n.AddExternalRoute(n.Name, proto.NewHostOnlySubnet(n.Address), defaultCost(n.Cost), ns.SendPacket)
	inst.nsreg.Add(n.Name, ns.PID())
	return inst, nil
}

func (n NamespaceCfg) Children() map[string]reconciler.ConfigItem {
	return nil
}

func defaultCost(cost float32) float32 {
	if cost <= 0 {
		return 1.0
	}
	return cost
}
