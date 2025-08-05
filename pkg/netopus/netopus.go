package netopus

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ghjm/connectopus/pkg/backends"
	"github.com/ghjm/connectopus/pkg/backends/backend_registry"
	"github.com/ghjm/connectopus/pkg/config"
	"github.com/ghjm/connectopus/pkg/netstack"
	"github.com/ghjm/connectopus/pkg/proto"
	"github.com/ghjm/connectopus/pkg/router"
	"github.com/ghjm/golib/pkg/syncro"
	"github.com/ghjm/golib/pkg/timerunner"
	log "github.com/sirupsen/logrus"
	"golang.org/x/exp/slices"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"math"
	"net"
	"strings"
	"time"
)

// netopus implements proto.Netopus
type netopus struct {
	ctx            context.Context
	addr           proto.IP
	name           string
	mtu            uint16
	stack          netstack.NetStack
	router         router.Router
	sessionInfo    syncro.Var[sessInfo]
	epoch          uint64
	sequence       syncro.Var[uint64]
	knownNodeInfo  syncro.Map[proto.IP, nodeInfo]
	externalRoutes syncro.Map[string, externalRouteInfo]
	externalNames  syncro.Map[string, externalNameInfo]
	updateSender   timerunner.TimeRunner
	oobPorts       syncro.Map[uint16, *OOBPacketConn]
	configInfo     syncro.Var[configInfo]
	newConfigFunc  func([]byte, []byte)
}

const (
	connectionStateInit byte = iota
	connectionStateConnecting
	connectionStateConnected
)

type configInfo struct {
	configVersion   time.Time
	configData      []byte
	configSignature []byte
}

// externalRouteInfo stores information about a route
type externalRouteInfo struct {
	dest               proto.Subnet
	cost               float32
	outgoingPacketFunc func(data []byte) error
}

// externalNameInfo stores information about a name
type externalNameInfo struct {
	ip       proto.IP
	fromNode string
}

// sessInfo stores information about sessions
type sessInfo map[proto.IP]*protoSession

// protoSession represents one running session of a protocol
type protoSession struct {
	n                  *netopus
	ctx                context.Context
	cancel             context.CancelFunc
	conn               backends.BackendConnection
	cost               float32
	readChan           chan proto.Msg
	remoteAddr         syncro.Var[proto.IP]
	connectionState    syncro.Var[byte]
	connStart          syncro.Var[time.Time]
	lastInit           time.Time
	oobRoutePacketConn syncro.Var[*OOBPacketConn]
	oobRouteListener   syncro.Var[net.Listener]
	oobRouteConn       syncro.Var[net.Conn]
}

// nodeInfo represents known information about a node
type nodeInfo struct {
	name          string
	epoch         uint64
	sequence      uint64
	configVersion time.Time
	lastUpdate    time.Time
}

type npOpts struct {
	mtu uint16
	ncf func([]byte, []byte)
}

// New constructs and returns a new network node on a given address
func New(ctx context.Context, addr proto.IP, name string, opts ...func(*npOpts)) (proto.Netopus, error) {
	o := &npOpts{
		mtu: 1500,
	}
	for _, opt := range opts {
		opt(o)
	}
	if len(addr) != net.IPv6len {
		return nil, fmt.Errorf("address must be IPv6")
	}
	stack, err := netstack.NewStackDefault(ctx, net.IP(addr), o.mtu)
	if err != nil {
		return nil, err
	}
	n := &netopus{
		ctx:         ctx,
		addr:        addr,
		name:        name,
		mtu:         o.mtu,
		stack:       stack,
		router:      router.New(ctx, addr, 100*time.Millisecond),
		sessionInfo: syncro.NewVar(make(sessInfo)),
		//nolint: gosec // UnixNano is always positive, therefore no overflow here
		epoch:         uint64(time.Now().UnixNano()),
		sequence:      syncro.Var[uint64]{},
		knownNodeInfo: syncro.Map[proto.IP, nodeInfo]{},
		oobPorts:      syncro.Map[uint16, *OOBPacketConn]{},
		newConfigFunc: o.ncf,
	}
	go func() {
		routerUpdateChan := n.router.SubscribeUpdates()
		defer n.router.UnsubscribeUpdates(routerUpdateChan)
		var lastRouteMsg string
		for {
			select {
			case <-ctx.Done():
				return
			case u := <-routerUpdateChan:
				conns := make([]string, 0, len(u))
				for k, v := range u {
					conns = append(conns, fmt.Sprintf("%s via %s", k, v))
				}
				slices.Sort(conns)
				var routeLogMsg string
				if len(conns) > 0 {
					routeLogMsg = fmt.Sprintf("%s: routing update: %v", n.addr.String(), strings.Join(conns, ", "))
				} else {
					routeLogMsg = fmt.Sprintf("%s: routing update: no routes", n.addr.String())
				}
				if routeLogMsg != lastRouteMsg {
					log.Info(routeLogMsg)
				} else {
					log.Debug(routeLogMsg)
				}
				lastRouteMsg = routeLogMsg
				n.sessionInfo.WorkWithReadOnly(func(s sessInfo) {
					for _, p := range s {
						p.n.updateSender.RunWithin(100 * time.Millisecond)
					}
				})
			}
		}
	}()
	go n.netStackReadLoop()
	n.updateSender = timerunner.New(ctx,
		func() {
			log.Debugf("%s: sending routing update", n.addr.String())
			ru, uerr := n.generateRoutingUpdate()
			if uerr != nil {
				log.Errorf("error generating routing update: %s", uerr)
				return
			}
			n.router.UpdateNode(n.addr, ru.Connections)
			n.floodRoutingUpdate(ru, nil)
		},
		timerunner.Periodic(10*time.Second))
	go n.monitorDeadNodes()
	return n, nil
}

func WithMTU(mtu uint16) func(*npOpts) {
	return func(o *npOpts) {
		o.mtu = mtu
	}
}

func WithNewConfigFunc(ncf func([]byte, []byte)) func(*npOpts) {
	return func(o *npOpts) {
		o.ncf = ncf
	}
}

// LeastMTU gives the smallest MTU of any backend defined on a node.  If there are no backends, defaultMTU is returned.
func LeastMTU(node config.Node, defaultMTU uint16) uint16 {
	mtu := uint16(math.MaxUint16)
	for _, b := range node.Backends {
		spec := backend_registry.LookupBackend(b.GetString("type", ""))
		if spec != nil && spec.MTU < mtu {
			mtu = spec.MTU
		}
	}
	if mtu == math.MaxUint16 {
		mtu = defaultMTU
	}
	return mtu
}

// MTU returns the global MTU for the Netopus instance.  This may not be the same as the MTU for a given backend.
func (n *netopus) MTU() uint16 {
	return n.mtu
}

// backendReadLoop reads messages from the connection and sends them to a channel
func (p *protoSession) backendReadLoop() {
	defer p.cancel()
	for {
		data, err := p.conn.ReadMessage()
		if p.ctx.Err() != nil {
			return
		}
		if err != nil {
			log.Warnf("protocol read error: %s", err)
			return
		}
		select {
		case <-p.ctx.Done():
			return
		case p.readChan <- data:
		}
	}
}

func (p *protoSession) routeReader() {
	defer p.cancel()
	orc := p.oobRouteConn.Get()
	reader := bufio.NewReader(orc)
	for {
		msgBytes, err := reader.ReadBytes('\n')
		if p.ctx.Err() != nil {
			return
		}
		var msg proto.RoutingUpdate
		if err == nil {
			err = json.Unmarshal(msgBytes, &msg)
		}
		if err != nil {
			log.Errorf("OOB route reader failure: %s", err)
			return
		}
		viaNode := p.remoteAddr.Get()
		log.Debugf("%s: received routing update %d from %s via %s", p.n.addr.String(),
			msg.UpdateSequence, msg.Origin.String(), viaNode)
		if p.n.handleRoutingUpdate(&msg) {
			p.n.router.UpdateNode(msg.Origin, msg.Connections)
			p.n.externalNames.WorkWith(func(_en *map[string]externalNameInfo) {
				en := *_en
				for k, v := range msg.Names {
					r, ok := en[k]
					if ok && r.fromNode == "" {
						continue
					}
					en[k] = externalNameInfo{
						ip:       v,
						fromNode: msg.Origin.String(),
					}
				}
			})
			p.n.floodRoutingUpdate(&msg, &viaNode)
		}
	}
}

// sendInit sends an initialization message
func (p *protoSession) sendInit() {
	log.Debugf("%s: sending init message", p.n.addr.String())
	cfb := make([]byte, 8)
	n, err := rand.Read(cfb)
	if err == nil && n != 8 {
		err = fmt.Errorf("wrong number of random bytes read")
	}
	if err != nil {
		log.Errorf("error getting random number: %s", err)
		return
	}
	var im []byte
	im, err = (&proto.InitMsg{MyAddr: p.n.addr}).Marshal()
	if err == nil {
		err = p.conn.WriteMessage(im)
	}
	if err != nil {
		log.Warnf("error sending init message: %s", err)
		return
	}
	p.lastInit = time.Now()
}

// initSelect runs one instance of the main loop when the connection is not established
func (p *protoSession) initSelect() bool {
	if p.connectionState.Get() == connectionStateInit {
		p.sendInit()
		if p.conn.IsServer() {
			p.oobRouteListener.WorkWith(func(_li *net.Listener) {
				li := *_li
				if li == nil {
					opc, err := NewPacketConn(p.ctx, p.n.sendRoutingOOB, nil, p.n.addr, 0)
					if err != nil {
						opc = nil
						log.Errorf("%s: packet conn error: %s", p.n.addr.String(), err)
						p.cancel()
					}
					p.oobRoutePacketConn.Set(opc)
					li, err = ListenOOB(p.ctx, opc)
					if err != nil {
						li = nil
						log.Errorf("%s: OOB routing listen error: %s", p.n.addr.String(), err)
						p.cancel()
					}
				}
				*_li = li
			})
		}
	}
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-p.ctx.Done():
		return false
	case <-timer.C:
		return false
	case data := <-p.readChan:
		msgAny, err := data.Unmarshal()
		if err != nil {
			return false
		}
		switch msg := msgAny.(type) {
		case *proto.InitMsg:
			var proceed = false
			p.connectionState.WorkWith(func(state *byte) {
				if *state == connectionStateInit {
					proceed = true
					*state = connectionStateConnecting
				}
			})
			if !proceed {
				return false
			}
			log.Debugf("%s: connecting to %s", p.n.addr.String(), msg.MyAddr.String())
			p.remoteAddr.Set(msg.MyAddr)
			p.n.sessionInfo.WorkWith(func(s *sessInfo) {
				si := *s
				oldSess, ok := si[msg.MyAddr]
				if ok && oldSess != p {
					log.Infof("%s: closing old connection to %s", p.n.addr.String(), msg.MyAddr.String())
					oldSess.cancel()
				}
				si[msg.MyAddr] = p
				*s = si
			})
			routeConnCancelChan := make(chan struct{})
			routeConnTimer := time.NewTimer(5 * time.Second)
			go func() {
				select {
				case <-p.ctx.Done():
					routeConnTimer.Stop()
				case <-routeConnCancelChan:
					routeConnTimer.Stop()
				case <-routeConnTimer.C:
					p.cancel()
				}
			}()
			finalizeConn := func(orc net.Conn) {
				log.Debugf("%s: RoutingOOB connection succeeded to/from %s", p.n.addr.String(), msg.MyAddr.String())
				p.oobRouteConn.Set(orc)
				close(routeConnCancelChan)
				p.connectionState.Set(connectionStateConnected)
				p.connStart.Set(time.Now())
				log.Infof("%s: connected to %s", p.n.addr.String(), msg.MyAddr.String())
				go p.routeReader()
				p.n.updateSender.RunWithin(100 * time.Millisecond)
			}
			if p.conn.IsServer() {
				go func() {
					li := p.oobRouteListener.Get()
					if li != nil {
						var orc net.Conn
						log.Debugf("%s: RoutingOOB accepting connection from %s", p.n.addr.String(), msg.MyAddr.String())
						orc, err = li.Accept()
						if p.ctx.Err() != nil {
							p.connectionState.Set(connectionStateInit)
							return
						}
						if err != nil {
							log.Errorf("%s: RoutingOOB accept failed from %s: %s", p.n.addr.String(), msg.MyAddr.String(), err)
							p.cancel()
							p.connectionState.Set(connectionStateInit)
							return
						}
						finalizeConn(orc)
					}
				}()
			} else {
				go func() {
					startTime := time.Now()
					for {
						log.Debugf("%s: RoutingOOB dialing %s", p.n.addr.String(), msg.MyAddr.String())
						opc, err := NewPacketConn(p.ctx, p.n.sendRoutingOOB, nil, p.n.addr, 0)
						if err != nil {
							opc = nil
							log.Errorf("%s: packet conn error: %s", p.n.addr.String(), err)
							p.cancel()
						}
						var orc net.Conn
						orc, err = DialOOB(p.ctx, proto.OOBAddr{Host: msg.MyAddr, Port: 0}, opc)
						if err == nil {
							p.oobRoutePacketConn.Set(opc)
							finalizeConn(orc)
							return
						}
						log.Debugf("%s: RoutingOOB dialing %s: error %s", p.n.addr.String(), msg.MyAddr.String(), err)
						t := time.NewTimer(100 * time.Millisecond)
						select {
						case <-p.ctx.Done():
							t.Stop()
							p.connectionState.Set(connectionStateInit)
							return
						case <-t.C:
						}
						if time.Now().After(startTime.Add(5 * time.Second)) {
							log.Debugf("%s: RoutingOOB dialing %s: giving up", p.n.addr.String(), msg.MyAddr.String())
							p.connectionState.Set(connectionStateInit)
							return
						}
					}
				}()
			}
			return true
		case *proto.RouteOOBMessage:
			opc := p.oobRoutePacketConn.Get()
			if opc != nil {
				oobMsg := (*proto.OOBMessage)(msg)
				err := opc.IncomingPacket(oobMsg)
				if err != nil {
					log.Warnf("OOB error from %s:%d to %s:%d: %s",
						msg.SourceAddr, msg.SourcePort, msg.DestAddr, msg.DestPort, err)
				}
			}
			return true
		case *proto.KeepaliveMsg:
			kr := &proto.KeepaliveReplyMsg{}
			ka, _ := kr.Marshal()
			_ = p.conn.WriteMessage(ka)
			return true
		case *proto.KeepaliveReplyMsg:
			return true
		default:
			log.Debugf("%s: received non-init message while in init mode", p.n.addr.String())
		}
		return false
	}
}

// mainSelect runs one instance of the main loop when the connection is established
func (p *protoSession) mainSelect() bool {
	select {
	case <-p.ctx.Done():
		return false
	case data := <-p.readChan:
		msgAny, err := data.Unmarshal()
		if err != nil {
			log.Warnf("packet unmarshaling error: %s", err)
			return false
		}
		switch msg := msgAny.(type) {
		case []byte:
			err = p.n.SendPacket(data)
			if err != nil && !errors.Is(err, context.Canceled) {
				log.Warnf("packet sending error: %s", err)
			}
			return true
		case *proto.InitMsg:
			log.Debugf("%s: received init message from %s while in main loop", p.n.addr.String(),
				p.remoteAddr.Get().String())
			if time.Since(p.lastInit) > 2*time.Second {
				p.connectionState.Set(connectionStateInit)
			}
			return false
		case *proto.OOBMessage:
			err = p.n.SendOOB(msg)
			if err != nil {
				log.Warnf("OOB error from %s:%d to %s:%d: %s",
					msg.SourceAddr, msg.SourcePort, msg.DestAddr, msg.DestPort, err)
			}
			return true
		case *proto.RouteOOBMessage:
			opc := p.oobRoutePacketConn.Get()
			if opc != nil {
				oobMsg := (*proto.OOBMessage)(msg)
				err = opc.IncomingPacket(oobMsg)
				if err != nil {
					log.Warnf("OOB error from %s:%d to %s:%d: %s",
						msg.SourceAddr, msg.SourcePort, msg.DestAddr, msg.DestPort, err)
				}
			}
			return true
		case *proto.KeepaliveMsg:
			kr := &proto.KeepaliveReplyMsg{}
			ka, _ := kr.Marshal()
			_ = p.conn.WriteMessage(ka)
			return true
		case *proto.KeepaliveReplyMsg:
			return true
		}
	}
	return false
}

// protoLoop is the main protocol loop
func (p *protoSession) protoLoop() {
	defer func() {
		p.cancel()
	}()
	lastActivity := syncro.Var[time.Time]{}
	go func() {
		km := &proto.KeepaliveMsg{}
		ka, _ := km.Marshal()
		for {
			timer := time.NewTimer(time.Second)
			select {
			case <-p.ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
				lA := time.Since(lastActivity.Get())
				if lA > 7*time.Second {
					log.Warnf("%s: closing connection to %s due to inactivity", p.n.addr.String(), p.remoteAddr.Get().String())
					p.cancel()
				} else if lA > 3*time.Second {
					err := p.conn.WriteMessage(ka)
					if err != nil && p.ctx.Err() == nil {
						log.Warnf("keepalive write error: %s", err)
					}
				}
			}
		}
	}()
	for {
		var activity bool
		if p.connectionState.Get() == connectionStateConnected {
			activity = p.mainSelect()
		} else {
			activity = p.initSelect()
		}
		if activity {
			lastActivity.Set(time.Now())
		}
		if p.ctx.Err() != nil {
			return
		}
	}
}

// RunProtocol runs the Netopus protocol over a given backend connection
func (n *netopus) RunProtocol(ctx context.Context, cost float32, conn backends.BackendConnection) {
	if cost <= 0 {
		cost = 1.0
	}
	protoCtx, protoCancel := context.WithCancel(ctx)
	defer func() {
		protoCancel()
	}()
	p := &protoSession{
		n:        n,
		ctx:      protoCtx,
		cancel:   protoCancel,
		conn:     conn,
		cost:     cost,
		readChan: make(chan proto.Msg),
	}
	defer func() {
		n.sessionInfo.WorkWith(func(s *sessInfo) {
			si := *s
			for k, v := range si {
				if v == p {
					delete(si, k)
				}
			}
			*s = si
		})
		n.updateSender.RunWithin(100 * time.Millisecond)
	}()
	go p.backendReadLoop()
	go p.protoLoop()
	<-protoCtx.Done()
}

// netStackReadLoop reads packets from the netstack and processes them
func (n *netopus) netStackReadLoop() {
	readChan := n.stack.SubscribePackets()
	for {
		select {
		case <-n.ctx.Done():
			return
		case packet := <-readChan:
			err := n.SendPacket(packet)
			if n.ctx.Err() != nil {
				return
			}
			if err != nil {
				log.Warnf("protocol write error: %s", err)
				continue
			}
		}
	}
}

func (n *netopus) sendICMPv6Error(origPacket header.IPv6, icmpType header.ICMPv6Type, icmpCode header.ICMPv6Code) {
	extraSize := header.IPv6MinimumSize + 8
	if extraSize > len(origPacket) {
		extraSize = len(origPacket)
	}
	replyPkt := header.IPv6(make([]byte, header.IPv6MinimumSize+header.ICMPv6MinimumSize+extraSize))
	replyPkt.Encode(&header.IPv6Fields{
		// #nosec G115
		PayloadLength:     uint16(header.ICMPv6MinimumSize + extraSize),
		TransportProtocol: header.ICMPv6ProtocolNumber,
		HopLimit:          30,
		SrcAddr:           tcpip.AddrFromSlice([]byte(n.addr)),
		DstAddr:           origPacket.SourceAddress(),
	})
	replyICMP := header.ICMPv6(replyPkt.Payload())
	replyICMP.SetType(icmpType)
	replyICMP.SetCode(icmpCode)
	copy(replyICMP[header.ICMPv6MinimumSize:], origPacket[:extraSize])
	replyICMP.SetChecksum(header.ICMPv6Checksum(header.ICMPv6ChecksumParams{
		Header: replyICMP,
		Src:    replyPkt.SourceAddress(),
		Dst:    replyPkt.DestinationAddress(),
	}))
	_ = n.SendPacket(replyPkt)
}

// SendPacket sends a single IPv6 packet over the network.
func (n *netopus) SendPacket(packet []byte) error {
	if len(packet) < header.IPv6MinimumSize {
		return fmt.Errorf("malformed packet: too small")
	}
	ipv6Pkt := header.IPv6(packet)
	da := ipv6Pkt.DestinationAddress()
	dest := proto.IP((&da).AsSlice())
	if header.IsV6MulticastAddress(da) {
		return nil
	}

	// Check if we can deliver the packet locally
	if dest.Equal(n.addr) {
		return n.stack.SendPacket(packet)
	}

	// Packet is being forwarded, so check the hop count
	limit := ipv6Pkt.HopLimit()
	if limit <= 1 {
		// No good - return an ICMPv6 Hop Limit Exceeded
		n.sendICMPv6Error(ipv6Pkt, header.ICMPv6TimeExceeded, header.ICMPv6HopLimitExceeded)
		return nil
	}
	ipv6Pkt.SetHopLimit(limit - 1)

	// Check if this packet is destined to an external route
	var opf func([]byte) error
	n.externalRoutes.WorkWithReadOnly(func(routes map[string]externalRouteInfo) {
		for _, r := range routes {
			if r.dest.Contains(dest) {
				opf = r.outgoingPacketFunc
				break
			}
		}
	})
	if opf != nil {
		return opf(packet)
	}

	// Send the packet to the next hop
	nextHop, err := n.router.NextHop(dest)
	if err != nil {
		n.updateSender.RunWithin(100 * time.Millisecond)
		n.sendICMPv6Error(ipv6Pkt, header.ICMPv6DstUnreachable, header.ICMPv6NetworkUnreachable)
		return fmt.Errorf("no route to host: %s", dest.String())
	}
	var nextHopSess *protoSession
	n.sessionInfo.WorkWithReadOnly(func(s sessInfo) {
		si := s
		nextHopSess = si[nextHop]
	})
	if nextHopSess == nil {
		return fmt.Errorf("no connection to next hop %s", nextHop)
	}
	go func() {
		err := nextHopSess.conn.WriteMessage(packet)
		if err != nil && n.ctx.Err() == nil {
			log.Warnf("packet write error: %s", err)
		}
	}()
	return nil
}

// SendOOB sends a single OOB packet over the network.
func (n *netopus) SendOOB(msg *proto.OOBMessage) error {
	// Check if we can deliver the packet locally
	if msg.DestAddr.Equal(n.addr) {
		pc, ok := n.oobPorts.Get(msg.DestPort)
		if !ok {
			return fmt.Errorf("OOB port unreachable")
		}
		go func() {
			_ = pc.IncomingPacket(msg)
		}()
		return nil
	}

	// Packet is being forwarded, so check the hop count
	msg.Hops++
	if msg.Hops >= 30 {
		return fmt.Errorf("OOB message expired in transit")
	}

	// Send the packet to the next hop
	nextHop, err := n.router.NextHop(msg.DestAddr)
	if err != nil {
		nextHop = msg.DestAddr
	}
	var nextHopSess *protoSession
	n.sessionInfo.WorkWithReadOnly(func(s sessInfo) {
		si := s
		nextHopSess = si[nextHop]
	})
	if nextHopSess == nil {
		return fmt.Errorf("OOB no connection to next hop %s", nextHop)
	}
	var msgBytes []byte
	msgBytes, err = msg.Marshal()
	if err != nil {
		return fmt.Errorf("OOB marshal error: %w", err)
	}
	go func() {
		err := nextHopSess.conn.WriteMessage(msgBytes)
		if err != nil && n.ctx.Err() == nil {
			log.Warnf("packet write error: %s", err)
		}
	}()
	return nil
}

// sendRoutingOOB sends a single RoutingOOB packet to a connected peer.
func (n *netopus) sendRoutingOOB(msg *proto.OOBMessage) error {
	var peerSess *protoSession
	n.sessionInfo.WorkWithReadOnly(func(s sessInfo) {
		si := s
		peerSess = si[msg.DestAddr]
	})
	if peerSess == nil {
		log.Warnf("routingOOB packet to unknown destination %s", msg.DestAddr.String())
		return nil
	}
	msgBytes, err := (*proto.RouteOOBMessage)(msg).Marshal()
	if err != nil {
		return fmt.Errorf("RouteOOB marshal error: %w", err)
	}
	go func() {
		err := peerSess.conn.WriteMessage(msgBytes)
		if err != nil && n.ctx.Err() == nil {
			log.Warnf("packet write error: %s", err)
		}
	}()
	return nil
}

// floodRoutingUpdate forwards a message to all neighbors except one (likely the one we received it from)
func (n *netopus) floodRoutingUpdate(update *proto.RoutingUpdate, except *proto.IP) {
	n.sessionInfo.WorkWithReadOnly(func(s sessInfo) {
		for node, sess := range s {
			if except != nil && node == *except {
				continue
			}
			if sess.connectionState.Get() == connectionStateConnected {
				go func(sess *protoSession) {
					orc := sess.oobRouteConn.Get()
					if orc != nil {
						msgBytes, err := json.Marshal(update)
						if err == nil {
							msgBytes = append(msgBytes, '\n')
							_, err = orc.Write(msgBytes)
						}
						if err != nil && n.ctx.Err() == nil {
							log.Warnf("routing update send error: %s", err)
						}
					}
				}(sess)
			}
		}
	})
}

// generateRoutingUpdate produces a routing update suitable for being sent to peers.
func (n *netopus) generateRoutingUpdate() (*proto.RoutingUpdate, error) {
	conns := make(proto.RoutingConns)
	n.sessionInfo.WorkWithReadOnly(func(s sessInfo) {
		for _, sess := range s {
			if sess.connectionState.Get() == connectionStateConnected {
				conns[proto.NewSubnet(sess.remoteAddr.Get(), proto.CIDRMask(128, 128))] = sess.cost
			}
		}
	})
	n.externalRoutes.WorkWithReadOnly(func(routes map[string]externalRouteInfo) {
		for _, r := range routes {
			conns[r.dest] = r.cost
		}
	})
	names := make(map[string]proto.IP)
	n.externalNames.WorkWithReadOnly(func(m map[string]externalNameInfo) {
		for k, v := range m {
			names[k] = v.ip
		}
	})
	ci := n.configInfo.Get()
	up := &proto.RoutingUpdate{
		Origin:        n.addr,
		NodeName:      n.name,
		UpdateEpoch:   n.epoch,
		Connections:   conns,
		Names:         names,
		ConfigVersion: ci.configVersion,
	}
	if !n.checkConfigsConverged(ci.configVersion) {
		log.Debugf("%s: sending updated configuration", n.addr.String())
		up.ConfigData = ci.configData
		up.ConfigSignature = ci.configSignature
	}
	n.sequence.WorkWith(func(_seq *uint64) {
		seq := *_seq
		seq++
		up.UpdateSequence = seq
		*_seq = seq
	})
	return up, nil
}

// handleRoutingUpdate updates the local routing table and known nodes data.  Returns a bool indicating whether the
// update should be forwarded.
func (n *netopus) handleRoutingUpdate(ru *proto.RoutingUpdate) bool {
	if ru.Origin.String() == n.addr.String() {
		log.Debugf("%s: rejecting routing update because we are the origin", n.addr.String())
		return false
	}
	var retval bool
	n.knownNodeInfo.WorkWith(func(_knownNodeInfo *map[proto.IP]nodeInfo) {
		knownNodeInfo := *_knownNodeInfo
		ni, ok := (knownNodeInfo)[ru.Origin]
		if ok && (ru.UpdateEpoch < ni.epoch || (ru.UpdateEpoch == ni.epoch && ru.UpdateSequence < ni.sequence)) {
			log.Debugf("%s: ignoring outdated routing update", n.addr.String())
			return
		}
		if ru.UpdateEpoch == ni.epoch && ru.UpdateSequence == ni.sequence {
			log.Debugf("%s: ignoring duplicate routing update", n.addr.String())
			return
		}
		retval = true
		knownNodeInfo[ru.Origin] = nodeInfo{
			name:          ru.NodeName,
			epoch:         ru.UpdateEpoch,
			sequence:      ru.UpdateSequence,
			configVersion: ru.ConfigVersion,
			lastUpdate:    time.Now(),
		}
		if n.newConfigFunc != nil && len(ru.ConfigData) > 0 && len(ru.ConfigSignature) > 0 {
			ci := n.configInfo.Get()
			if ru.ConfigVersion.After(ci.configVersion) {
				log.Debugf("%s: received newer configuration from %s", n.addr.String(), ru.Origin)
				n.newConfigFunc(ru.ConfigData, ru.ConfigSignature)
			}
		}
	})
	return retval
}

// monitorDeadNodes prunes nodes that haven't had an update in a long time
func (n *netopus) monitorDeadNodes() {
	for {
		n.knownNodeInfo.WorkWith(func(_knownNodeInfo *map[proto.IP]nodeInfo) {
			knownNodeInfo := *_knownNodeInfo
			for node, info := range knownNodeInfo {
				if info.lastUpdate.Before(time.Now().Add(-time.Minute)) {
					log.Warnf("%s: removing dead node %s", n.addr.String(), node.String())
					delete(knownNodeInfo, node)
					n.router.RemoveNode(node)
				}
			}
		})
		select {
		case <-n.ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

// Status returns status of the Netopus instance
func (n *netopus) Status() *proto.Status {
	s := proto.Status{
		Name: n.name,
		Addr: n.addr,
	}
	s.NameToAddr = make(map[string]string)
	s.AddrToName = make(map[string]string)
	s.NameToAddr[n.name] = n.addr.String()
	s.AddrToName[n.addr.String()] = n.name
	n.knownNodeInfo.WorkWithReadOnly(func(kn map[proto.IP]nodeInfo) {
		for k, v := range kn {
			s.NameToAddr[v.name] = k.String()
			s.AddrToName[k.String()] = v.name
		}
	})
	s.RouterNodes = make(map[string]map[string]float32)
	{
		for k, v := range n.router.Nodes() {
			r := make(map[string]float32)
			for k2, v2 := range v {
				r[k2.String()] = v2
			}
			s.RouterNodes[k.String()] = r
		}
	}
	s.Sessions = make(map[string]proto.SessionStatus)
	n.sessionInfo.WorkWithReadOnly(func(si sessInfo) {
		for k, v := range si {
			s.Sessions[k.String()] = proto.SessionStatus{
				Connected: v.connectionState.Get() == connectionStateConnected,
				ConnStart: v.connStart.Get(),
			}
		}
	})
	return &s
}

// DialTCP implements netstack.UserStack
func (n *netopus) DialTCP(addr net.IP, port uint16) (net.Conn, error) {
	return n.stack.DialTCP(addr, port)
}

// DialContextTCP implements netstack.UserStack
func (n *netopus) DialContextTCP(ctx context.Context, addr net.IP, port uint16) (net.Conn, error) {
	return n.stack.DialContextTCP(ctx, addr, port)
}

// ListenTCP implements netstack.UserStack
func (n *netopus) ListenTCP(port uint16) (net.Listener, error) {
	return n.stack.ListenTCP(port)
}

// DialUDP implements netstack.UserStack
func (n *netopus) DialUDP(lport uint16, addr net.IP, rport uint16) (netstack.UDPConn, error) {
	return n.stack.DialUDP(lport, addr, rport)
}

// AddExternalRoute implements ExternalRouter
func (n *netopus) AddExternalRoute(name string, dest proto.Subnet, cost float32, outgoingPacketFunc func([]byte) error) {
	if cost <= 0 {
		cost = 1.0
	}
	n.externalRoutes.Set(name, externalRouteInfo{
		dest:               dest,
		cost:               cost,
		outgoingPacketFunc: outgoingPacketFunc,
	})
	n.updateSender.RunWithin(500 * time.Millisecond)
}

// DelExternalRoute implements ExternalRouter
func (n *netopus) DelExternalRoute(name string) {
	n.externalRoutes.Delete(name)
	n.updateSender.RunWithin(500 * time.Millisecond)
}

// SubscribeUpdates implements ExternalRouter
func (n *netopus) SubscribeUpdates() <-chan proto.RoutingPolicy {
	return n.router.SubscribeUpdates()
}

// UnsubscribeUpdates implements ExternalRouter
func (n *netopus) UnsubscribeUpdates(ch <-chan proto.RoutingPolicy) {
	n.router.UnsubscribeUpdates(ch)
}

// AddExternalName implements NameService
func (n *netopus) AddExternalName(name string, ip proto.IP) {
	n.externalNames.Set(name, externalNameInfo{
		ip:       ip,
		fromNode: "",
	})
	n.updateSender.RunWithin(500 * time.Millisecond)
}

// DelExternalName implements NameService
func (n *netopus) DelExternalName(name string) {
	n.externalNames.Delete(name)
	n.updateSender.RunWithin(500 * time.Millisecond)
}

// LookupName implements NameService
func (n *netopus) LookupName(name string) proto.IP {
	if name == n.name {
		a := n.addr
		return a
	}
	var foundIP proto.IP
	n.knownNodeInfo.WorkWithReadOnly(func(kn map[proto.IP]nodeInfo) {
		for k, v := range kn {
			if v.name == name {
				foundIP = k
				return
			}
		}
	})
	if len(foundIP) > 0 {
		return foundIP
	}
	v, ok := n.externalNames.Get(name)
	if ok {
		return v.ip
	}
	return ""
}

// LookupIP implements NameService
func (n *netopus) LookupIP(ip proto.IP) string {
	if ip.Equal(n.addr) {
		return n.name
	}
	ni, ok := n.knownNodeInfo.Get(ip)
	if ok {
		return ni.name
	}
	foundName := ""
	n.externalNames.WorkWithReadOnly(func(en map[string]externalNameInfo) {
		for k, v := range en {
			if v.ip == ip {
				foundName = k
				return
			}
		}
	})
	return foundName
}

func (n *netopus) Addr() proto.IP {
	return n.addr
}

func (n *netopus) UpdateConfig(cfg []byte, sig []byte, version time.Time) {
	n.configInfo.Set(configInfo{
		configVersion:   version,
		configData:      cfg,
		configSignature: sig,
	})
	n.updateSender.RunWithin(100 * time.Millisecond)
}

func (n *netopus) checkConfigsConverged(myVersion time.Time) bool {
	converged := true
	n.knownNodeInfo.WorkWithReadOnly(func(m map[proto.IP]nodeInfo) {
		for _, v := range m {
			if v.configVersion.Before(myVersion) {
				converged = false
				return
			}
		}
	})
	return converged
}

func (n *netopus) WaitForConfigConvergence(ctx context.Context) error {
	myVersion := n.configInfo.Get().configVersion
	for {
		t := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
		if n.checkConfigsConverged(myVersion) {
			return nil
		}
	}
}
