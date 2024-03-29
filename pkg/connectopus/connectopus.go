package connectopus

import (
	"bufio"
	"context"
	"fmt"
	"github.com/ghjm/connectopus/internal/ui_embed"
	"github.com/ghjm/connectopus/pkg/backends/backend_registry"
	"github.com/ghjm/connectopus/pkg/config"
	"github.com/ghjm/connectopus/pkg/cpctl"
	"github.com/ghjm/connectopus/pkg/dns"
	"github.com/ghjm/connectopus/pkg/links/netns"
	"github.com/ghjm/connectopus/pkg/links/tun"
	"github.com/ghjm/connectopus/pkg/netopus"
	"github.com/ghjm/connectopus/pkg/proto"
	"github.com/ghjm/connectopus/pkg/services"
	"github.com/ghjm/golib/pkg/reconciler"
	"github.com/ghjm/golib/pkg/ssh_jwt"
	"github.com/golang-jwt/jwt/v4"
	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh/agent"
	"io"
	"net"
	"os"
	"os/exec"
	"path"
	"reflect"
	"sync"
	"time"
)

func SignConfig(keyFile, keyText string, cfg config.Config, updateTime bool) ([]byte, []byte, error) {
	a, err := ssh_jwt.GetSSHAgent(keyFile)
	if err != nil {
		return nil, nil, fmt.Errorf("error initializing SSH agent: %w", err)
	}
	defer func() {
		_ = a.Close()
	}()
	var keys []*agent.Key
	keys, err = ssh_jwt.GetMatchingKeys(a, keyText)
	if err != nil {
		return nil, nil, fmt.Errorf("error listing SSH keys: %w", err)
	}
	if len(keys) == 0 {
		return nil, nil, cpctl.ErrNoSSHKeys
	}
	if len(keys) > 1 {
		return nil, nil, cpctl.ErrMultipleSSHKeys
	}
	key := proto.MarshalablePublicKey{
		PublicKey: keys[0],
		Comment:   keys[0].Comment,
	}
	if updateTime {
		cfg.Global.LastUpdated = time.Now()
	}
	var cfgData []byte
	cfgData, err = cfg.Marshal()
	if err != nil {
		return nil, nil, fmt.Errorf("error marshaling config: %w", err)
	}
	var sig string
	sig, err = ssh_jwt.SignSSH(string(cfgData), "connectopus", key.String(), a)
	if err != nil {
		return nil, nil, fmt.Errorf("error signing config data: %w", err)
	}
	return cfgData, []byte(sig), nil
}

func ParseAndCheckConfig(cfgData, signature []byte, authKeys []proto.MarshalablePublicKey) (*config.Config, error) {
	cfg := config.Config{}
	err := cfg.Unmarshal(cfgData)
	if err != nil {
		return nil, fmt.Errorf("error parsing config file: %w", err)
	}
	if authKeys == nil {
		authKeys = cfg.Global.AuthorizedKeys
	}
	for _, authKey := range authKeys {
		err = ssh_jwt.VerifySSHSignature(string(cfgData), string(signature), "connectopus", authKey.String())
		if err == nil {
			return &cfg, nil
		}
	}
	return nil, fmt.Errorf("configuration signature check failed")
}

type Node reconciler.RunningItem

func LoadConfig(datadir string) (*config.Config, error) {
	data, err := os.ReadFile(path.Join(datadir, "config.yml"))
	if err != nil {
		return nil, fmt.Errorf("error loading config file: %w", err)
	}

	var sig []byte
	sig, err = os.ReadFile(path.Join(datadir, "config.sig"))
	if err != nil {
		return nil, fmt.Errorf("error reading signature file: %w", err)
	}

	return ParseAndCheckConfig(data, sig, nil)
}

func RunNode(ctx context.Context, datadir string, identity string) (*Node, error) {
	if identity == "" {
		return nil, fmt.Errorf("must provide an identity")
	}
	if datadir == "" {
		datadirs, err := config.FindDataDirs(identity)
		if err != nil {
			return nil, fmt.Errorf("error finding data dir: %w", err)
		}
		if len(datadirs) != 1 {
			return nil, fmt.Errorf("failed to find data dir for node")
		}
		datadir = datadirs[0]
	}
	cfg, err := LoadConfig(datadir)
	if err != nil {
		return nil, fmt.Errorf("error loading config: %w", err)
	}
	ri := reconciler.NewRootRunningItem(ctx, identity)
	nodeCfg := NodeCfg{
		Config:   cfg,
		identity: identity,
		datadir:  datadir,
	}
	ri.Reconcile(nodeCfg)
	return (*Node)(ri), ri.Status()
}

func (n *Node) ParseAndCheckConfig(cfgData []byte, signature []byte) (*config.Config, error) {
	ri := (*reconciler.RunningItem)(n)
	nodeCfg, ok := ri.Config().(NodeCfg)
	if !ok {
		return nil, fmt.Errorf("running instance config is wrong type")
	}
	return ParseAndCheckConfig(cfgData, signature, nodeCfg.Global.AuthorizedKeys)
}

func (n *Node) ReconcileNode(_ []byte, _ []byte, cfg *config.Config) error {
	ri := (*reconciler.RunningItem)(n)
	nodeCfg, ok := ri.Config().(NodeCfg)
	if !ok {
		return fmt.Errorf("running instance config is wrong type")
	}
	nodeCfg.Config = cfg
	ri.Reconcile(nodeCfg)
	return ri.Status()
}

type nodeInstance struct {
	cfg      *config.Config
	identity string
	node     *config.Node
	n        proto.Netopus
	nsreg    *netns.Registry
	ri       *reconciler.RunningItem
	datadir  string
}

func (ni *nodeInstance) NewConfig(config []byte, signature []byte) error {
	cfg, err := ParseAndCheckConfig(config, signature, ni.cfg.Global.AuthorizedKeys)
	if err != nil {
		return err
	}
	err = os.WriteFile(path.Join(ni.datadir, "config.yml"), config, 0600)
	if err != nil {
		return fmt.Errorf("error writing config.yml: %w", err)
	}
	err = os.WriteFile(path.Join(ni.datadir, "config.sig"), signature, 0600)
	if err != nil {
		return fmt.Errorf("error writing config.sig: %w", err)
	}
	ni.n.UpdateConfig(config, signature, cfg.Global.LastUpdated)
	go func() {
		ctx, cancel := context.WithTimeout(ni.ri.Context(), 30*time.Second)
		defer cancel()
		err := ni.n.WaitForConfigConvergence(ctx)
		if err != nil {
			log.Errorf("timeout: configuration did not converge")
			// no return - we'll try to reconcile anyway since we have a valid command from the user
		}
		err = (*Node)(ni.ri).ReconcileNode(config, signature, cfg)
		if err != nil {
			log.Errorf("error applying new configuration: %s", err)
		}
	}()
	return nil
}

type NodeCfg struct {
	*config.Config
	identity string
	datadir  string
}

func (nc NodeCfg) ParentEqual(item reconciler.ConfigItem) bool {
	ci, ok := item.(NodeCfg)
	if !ok {
		return false
	}
	if ci.identity != nc.identity {
		return false
	}
	ciG := ci.Global
	ncG := nc.Global
	ciG.LastUpdated = time.Time{}
	ncG.LastUpdated = time.Time{}
	if !reflect.DeepEqual(ciG, ncG) {
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

func (nc NodeCfg) Start(ctx context.Context, ri *reconciler.RunningItem, done func()) (any, error) {
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
		datadir:  nc.datadir,
	}
	cfgData, err := os.ReadFile(path.Join(nc.datadir, "config.yml"))
	if err != nil {
		return nil, fmt.Errorf("error loading config.yml: %w", err)
	}
	var sigData []byte
	sigData, err = os.ReadFile(path.Join(nc.datadir, "config.sig"))
	if err != nil {
		return nil, fmt.Errorf("error loading config.sig: %w", err)
	}
	inst.n, err = netopus.New(ctx, node.Address, nc.identity, netopus.WithMTU(netopus.LeastMTU(node, 1500)),
		netopus.WithNewConfigFunc(func(config []byte, signature []byte) {
			err := inst.NewConfig(config, signature)
			if err != nil {
				log.Warnf("error updating configuration: %s", err)
				return
			}
		}))
	if err != nil {
		return nil, err
	}
	inst.n.UpdateConfig(cfgData, sigData, nc.Config.Global.LastUpdated)
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

func (c CpctlCfg) Start(ctx context.Context, ri *reconciler.RunningItem, done func()) (any, error) {
	parentInst := ri.Parent().Instance().(*nodeInstance)
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
				cfg := parentInst.ri.Config()
				ncfg, ncOK := cfg.(NodeCfg)
				if !ncOK {
					return nil
				}
				return ncfg.Config
			},
			GetNetopus:          func() proto.Netopus { return parentInst.n },
			GetNsReg:            func() *netns.Registry { return parentInst.nsreg },
			GetReconcilerStatus: func() error { return parentInst.ri.Status() },
			UpdateNodeConfig:    parentInst.NewConfig,
		},
		SigningMethod: sm,
	}
	wg := sync.WaitGroup{}
	{
		li, err := parentInst.n.ListenOOB(ctx, ui_embed.ProxyPortNo)
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
		socketFile, err := config.ExpandFilename(parentInst.identity, c.SocketFile)
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
	return nil, nil
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

func (d DnsCfg) Start(ctx context.Context, ri *reconciler.RunningItem, done func()) (any, error) {
	parentInst := ri.Parent().Instance().(*nodeInstance)
	if d.Disable {
		return nil, nil
	}

	pc, err := parentInst.n.DialUDP(53, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("udp listener error: %w", err)
	}

	var li net.Listener
	li, err = parentInst.n.ListenTCP(53)
	if err != nil {
		_ = pc.Close()
		return nil, fmt.Errorf("tcp listener error: %w", err)
	}

	srv := dns.Server{
		Domain:     parentInst.cfg.Global.Domain,
		PacketConn: pc,
		Listener:   li,
		LookupName: parentInst.n.LookupName,
		LookupIP:   parentInst.n.LookupIP,
	}

	err = srv.Run(ctx)
	if err != nil {
		_ = pc.Close()
		_ = li.Close()
		return nil, err
	}

	go func() {
		<-ctx.Done()
		_ = pc.Close()
		_ = li.Close()
		done()
	}()

	return nil, nil
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

func (b BackendCfg) Start(ctx context.Context, ri *reconciler.RunningItem, done func()) (any, error) {
	parentInst := ri.Parent().Instance().(*nodeInstance)
	p := config.Params(b)
	backendType := p.GetString("type", "")
	backendCost, err := p.GetFloat32("cost", 0)
	if err != nil {
		return nil, fmt.Errorf("backend configuration error: %w", err)
	}
	err = backend_registry.RunBackend(ctx, parentInst.n, backendType, defaultCost(backendCost), p)
	if err != nil {
		return nil, fmt.Errorf("error initializing backend: %w", err)
	}
	go func() {
		<-ctx.Done()
		done()
	}()
	return nil, nil
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

func (s ServiceCfg) Start(ctx context.Context, ri *reconciler.RunningItem, done func()) (any, error) {
	parentInst := ri.Parent().Instance().(*nodeInstance)
	_, err := services.RunService(ctx, parentInst.n, config.Service(s))
	if err != nil {
		return nil, fmt.Errorf("error initializing service: %w", err)
	}
	go func() {
		<-ctx.Done()
		done()
	}()
	return nil, nil
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

func (t TunDevCfg) Start(ctx context.Context, ri *reconciler.RunningItem, done func()) (any, error) {
	parentInst := ri.Parent().Instance().(*nodeInstance)
	tunLink, err := tun.New(ctx, t.DeviceName, net.IP(t.Address), parentInst.cfg.Global.Subnet.AsIPNet(), parentInst.n.MTU())
	if err != nil {
		return nil, fmt.Errorf("error initializing tunnel: %w", err)
	}
	tunCh := tunLink.SubscribePackets()
	parentInst.n.AddExternalRoute(ri.Name(), proto.NewHostOnlySubnet(t.Address), defaultCost(t.Cost), tunLink.SendPacket)
	parentInst.n.AddExternalName(ri.Name(), t.Address)
	go func() {
		for {
			select {
			case <-ctx.Done():
				parentInst.n.DelExternalName(ri.Name())
				parentInst.n.DelExternalRoute(ri.Name())
				done()
				return
			case packet := <-tunCh:
				_ = parentInst.n.SendPacket(packet)
			}
		}
	}()
	return nil, nil
}

func (t TunDevCfg) Children() map[string]reconciler.ConfigItem {
	return nil
}

func (t TunDevCfg) Type() string {
	return "tundev"
}

type NamespaceCfg config.Namespace

type namespaceInstance struct {
	ni *nodeInstance
	ns *netns.Link
}

func (nc NamespaceCfg) ParentEqual(item reconciler.ConfigItem) bool {
	ci, ok := item.(NamespaceCfg)
	if !ok {
		return false
	}
	return ci.Address == nc.Address && ci.Cost == nc.Cost
}

func (nc NamespaceCfg) Start(ctx context.Context, ri *reconciler.RunningItem, done func()) (any, error) {
	parentInst := ri.Parent().Instance().(*nodeInstance)
	ns, err := netns.New(ctx, net.IP(nc.Address), parentInst.cfg.Global.Domain, parentInst.n.Addr().String(), netns.WithMTU(parentInst.n.MTU()))
	if err != nil {
		return nil, fmt.Errorf("error initializing namespace: %w", err)
	}
	nsCh := ns.SubscribePackets()
	parentInst.n.AddExternalRoute(ri.Name(), proto.NewHostOnlySubnet(nc.Address), defaultCost(nc.Cost), ns.SendPacket)
	parentInst.n.AddExternalName(ri.Name(), nc.Address)
	parentInst.nsreg.Add(ri.Name(), ns.PID())
	go func() {
		for {
			select {
			case <-ctx.Done():
				ns.UnsubscribePackets(nsCh)
				parentInst.nsreg.Del(ri.Name())
				parentInst.n.DelExternalName(ri.Name())
				parentInst.n.DelExternalRoute(ri.Name())
				done()
				return
			case packet := <-nsCh:
				_ = parentInst.n.SendPacket(packet)
			}
		}
	}()
	return &namespaceInstance{
		ni: parentInst,
		ns: ns,
	}, nil
}

func (nc NamespaceCfg) Children() map[string]reconciler.ConfigItem {
	children := make(map[string]reconciler.ConfigItem)
	for name, svc := range nc.Services {
		children[name] = NamespaceServiceCfg{
			command: svc.Command,
		}
	}
	return children
}

func (nc NamespaceCfg) Type() string {
	return "namespace"
}

type NamespaceServiceCfg struct {
	command string
}

func (nsc NamespaceServiceCfg) ParentEqual(item reconciler.ConfigItem) bool {
	ci, ok := item.(NamespaceServiceCfg)
	if !ok {
		return false
	}
	return reflect.DeepEqual(ci, nsc)
}

func (nsc NamespaceServiceCfg) Start(ctx context.Context, ri *reconciler.RunningItem, done func()) (any, error) {
	parentInst := ri.Parent().Instance().(*namespaceInstance)
	var stderrPipe io.ReadCloser
	cmd, err := parentInst.ns.RunInNamespace(ctx, nsc.command, func(c *exec.Cmd) error {
		var err error
		stderrPipe, err = c.StderrPipe()
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("error running command: %w", err)
	}
	go func() {
		r := bufio.NewReader(stderrPipe)
		for {
			s, err := r.ReadString('\n')
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				return
			}
			log.Warnf("%s wrote to stderr: %s", ri.QualifiedName(), s)
		}
	}()
	go func() {
		err := cmd.Wait()
		if err != nil && ctx.Err() == nil {
			log.Errorf("%s exited unexpectedly: %s", ri.QualifiedName(), err)
			ri.SetFailed(err)
		}
		done()
	}()
	return nil, nil
}

func (nsc NamespaceServiceCfg) Children() map[string]reconciler.ConfigItem {
	return nil
}

func (nsc NamespaceServiceCfg) Type() string {
	return "namespace_service"
}

func defaultCost(cost float32) float32 {
	if cost <= 0 {
		return 1.0
	}
	return cost
}
