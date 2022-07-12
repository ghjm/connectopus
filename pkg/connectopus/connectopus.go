package connectopus

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/pkg/backends/backend_registry"
	"github.com/ghjm/connectopus/pkg/config"
	"github.com/ghjm/connectopus/pkg/cpctl"
	"github.com/ghjm/connectopus/pkg/dns"
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
	"sync"
	"time"
)

func RunNode(ctx context.Context, cfgData []byte, identity string) (*reconciler.RunningItem, error) {
	cfg, err := config.ParseConfig(cfgData)
	if err != nil {
		return nil, fmt.Errorf("error parsing config: %w", err)
	}
	ri := reconciler.NewRootRunningItem(ctx, identity)
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

type nodeInstance struct {
	cfg      *config.Config
	identity string
	node     *config.Node
	n        proto.Netopus
	nsreg    *netns.Registry
	ri       *reconciler.RunningItem
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

func (nc NodeCfg) Start(ctx context.Context, name string, instance any, done func()) (any, error) {
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
	go func() {
		<-ctx.Done()
		time.Sleep(time.Second)
		done()
	}()
	return inst, nil
}

func (nc NodeCfg) Children() map[string]reconciler.ConfigItem {
	node, ok := nc.Config.Nodes[nc.identity]
	if !ok {
		return nil
	}
	children := make(map[string]reconciler.ConfigItem)
	children["cpctl"] = CpctlCfg(node.Cpctl)
	children["dns"] = DnsCfg(node.Dns)
	for k, v := range node.Backends {
		children[k] = BackendCfg(v)
	}
	for k, v := range node.Namespaces {
		children[k] = NamespaceCfg(v)
	}
	for k, v := range node.Services {
		children[k] = ServiceCfg(v)
	}
	for k, v := range node.TunDevs {
		children[k] = TunDevCfg(v)
	}
	for k, v := range node.Namespaces {
		children[k] = NamespaceCfg(v)
	}
	return children
}

func (nc NodeCfg) Type() string {
	return "node"
}

type CpctlCfg config.Cpctl

func (c CpctlCfg) ParentEqual(item reconciler.ConfigItem) bool {
	ci, ok := item.(CpctlCfg)
	if !ok {
		return false
	}
	return reflect.DeepEqual(ci, c)
}

func (c CpctlCfg) Start(ctx context.Context, name string, instance any, done func()) (any, error) {
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
	wg := sync.WaitGroup{}
	{
		li, err := inst.n.ListenOOB(ctx, cpctl.ProxyPortNo)
		if err != nil {
			return nil, fmt.Errorf("error initializing cpctl proxy listener: %w", err)
		}
		wg.Add(1)
		go func() {
			<-ctx.Done()
			_ = li.Close()
			wg.Done()
		}()
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
		var li net.Listener
		li, err = csrv.ServeUnix(ctx, socketFile)
		if err != nil {
			return nil, fmt.Errorf("error running socket server: %w", err)
		}
		wg.Add(1)
		go func() {
			<-ctx.Done()
			_ = li.Close()
			wg.Done()
		}()
	}
	if c.Port != 0 {
		lc := net.ListenConfig{}
		li, err := lc.Listen(ctx, "tcp", fmt.Sprintf(":%d", c.Port))
		if err != nil {
			return nil, fmt.Errorf("error initializing cpctl web server: %w", err)
		}
		err = csrv.ServeHTTP(ctx, li)
		if err != nil {
			_ = li.Close()
			return nil, fmt.Errorf("error running cpctl web server: %w", err)
		}
		wg.Add(1)
		go func() {
			<-ctx.Done()
			_ = li.Close()
			wg.Done()
		}()
	}
	go func() {
		<-ctx.Done()
		wg.Wait()
		done()
	}()
	return inst, nil
}

func (c CpctlCfg) Children() map[string]reconciler.ConfigItem {
	return nil
}

func (c CpctlCfg) Type() string {
	return "cpctl"
}

type DnsCfg config.Dns

func (d DnsCfg) ParentEqual(item reconciler.ConfigItem) bool {
	ci, ok := item.(DnsCfg)
	if !ok {
		return false
	}
	return reflect.DeepEqual(ci, d)
}

func (d DnsCfg) Start(ctx context.Context, name string, instance any, done func()) (any, error) {
	inst, ok := instance.(*nodeInstance)
	if !ok {
		return nil, fmt.Errorf("error retrieving instance: bad type")
	}

	if d.Disable {
		return inst, nil
	}

	pc, err := inst.n.DialUDP(53, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("udp listener error: %s", err)
	}

	var li net.Listener
	li, err = inst.n.ListenTCP(53)
	if err != nil {
		_ = pc.Close()
		return nil, fmt.Errorf("tcp listener error: %s", err)
	}

	srv := dns.Server{
		Domain:     inst.cfg.Global.Domain,
		PacketConn: pc,
		Listener:   li,
		LookupName: inst.n.LookupName,
		LookupIP:   inst.n.LookupIP,
	}

	err = srv.Run(ctx)
	if err != nil {
		_ = pc.Close()
		_ = li.Close()
		return nil, err
	}

	return inst, nil
}

func (d DnsCfg) Children() map[string]reconciler.ConfigItem {
	return nil
}

func (d DnsCfg) Type() string {
	return "dns"
}

type BackendCfg config.Params

func (b BackendCfg) ParentEqual(item reconciler.ConfigItem) bool {
	ci, ok := item.(BackendCfg)
	if !ok {
		return false
	}
	return reflect.DeepEqual(ci, b)
}

func (b BackendCfg) Start(ctx context.Context, name string, instance any, done func()) (any, error) {
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
	go func() {
		<-ctx.Done()
		done()
	}()
	return inst, nil
}

func (b BackendCfg) Children() map[string]reconciler.ConfigItem {
	return nil
}

func (b BackendCfg) Type() string {
	return "backend"
}

type ServiceCfg config.Service

func (s ServiceCfg) ParentEqual(item reconciler.ConfigItem) bool {
	ci, ok := item.(ServiceCfg)
	if !ok {
		return false
	}
	return reflect.DeepEqual(ci, s)
}

func (s ServiceCfg) Start(ctx context.Context, name string, instance any, done func()) (any, error) {
	inst, ok := instance.(*nodeInstance)
	if !ok {
		return nil, fmt.Errorf("error retrieving instance: bad type")
	}
	_, err := services.RunService(ctx, inst.n, config.Service(s))
	if err != nil {
		return nil, fmt.Errorf("error initializing service: %w", err)
	}
	go func() {
		<-ctx.Done()
		done()
	}()
	return inst, nil
}

func (s ServiceCfg) Children() map[string]reconciler.ConfigItem {
	return nil
}

func (s ServiceCfg) Type() string {
	return "service"
}

type TunDevCfg config.TunDev

func (t TunDevCfg) ParentEqual(item reconciler.ConfigItem) bool {
	ci, ok := item.(TunDevCfg)
	if !ok {
		return false
	}
	return reflect.DeepEqual(ci, t)
}

func (t TunDevCfg) Start(ctx context.Context, name string, instance any, done func()) (any, error) {
	inst, ok := instance.(*nodeInstance)
	if !ok {
		return nil, fmt.Errorf("error retrieving instance: bad type")
	}
	tunLink, err := tun.New(ctx, t.DeviceName, net.IP(t.Address), inst.cfg.Global.Subnet.AsIPNet(), inst.n.MTU())
	if err != nil {
		return nil, fmt.Errorf("error initializing tunnel: %w", err)
	}
	tunCh := tunLink.SubscribePackets()
	inst.n.AddExternalRoute(name, proto.NewHostOnlySubnet(t.Address), defaultCost(t.Cost), tunLink.SendPacket)
	inst.n.AddExternalName(name, t.Address)
	go func() {
		for {
			select {
			case <-ctx.Done():
				tunLink.UnsubscribePackets(tunCh)
				inst.n.DelExternalName(name)
				inst.n.DelExternalRoute(name)
				done()
				return
			case packet := <-tunCh:
				_ = inst.n.SendPacket(packet)
			}
		}
	}()
	return inst, nil
}

func (t TunDevCfg) Children() map[string]reconciler.ConfigItem {
	return nil
}

func (t TunDevCfg) Type() string {
	return "tundev"
}

type NamespaceCfg config.Namespace

func (nc NamespaceCfg) ParentEqual(item reconciler.ConfigItem) bool {
	ci, ok := item.(NamespaceCfg)
	if !ok {
		return false
	}
	return reflect.DeepEqual(ci, nc)
}

func (nc NamespaceCfg) Start(ctx context.Context, name string, instance any, done func()) (any, error) {
	inst, ok := instance.(*nodeInstance)
	if !ok {
		return nil, fmt.Errorf("error retrieving instance: bad type")
	}
	ns, err := netns.New(ctx, net.IP(nc.Address), inst.cfg.Global.Domain, inst.n.Addr().String(), netns.WithMTU(inst.n.MTU()))
	if err != nil {
		return nil, fmt.Errorf("error initializing namespace: %w", err)
	}
	nsCh := ns.SubscribePackets()
	inst.n.AddExternalRoute(name, proto.NewHostOnlySubnet(nc.Address), defaultCost(nc.Cost), ns.SendPacket)
	inst.n.AddExternalName(name, nc.Address)
	inst.nsreg.Add(name, ns.PID())
	go func() {
		for {
			select {
			case <-ctx.Done():
				ns.UnsubscribePackets(nsCh)
				inst.nsreg.Del(name)
				inst.n.DelExternalName(name)
				inst.n.DelExternalRoute(name)
				done()
				return
			case packet := <-nsCh:
				_ = inst.n.SendPacket(packet)
			}
		}
	}()
	return inst, nil
}

func (nc NamespaceCfg) Children() map[string]reconciler.ConfigItem {
	return nil
}

func (nc NamespaceCfg) Type() string {
	return "namespace"
}

func defaultCost(cost float32) float32 {
	if cost <= 0 {
		return 1.0
	}
	return cost
}
