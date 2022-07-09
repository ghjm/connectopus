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
	"github.com/ghjm/connectopus/pkg/x/syncro"
	"github.com/ghjm/connectopus/pkg/x/timerunner"
	log "github.com/sirupsen/logrus"
	"golang.org/x/exp/slices"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"math"
	"net"
	"strings"
	"syscall"
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
	externalRoutes syncro.Var[[]externalRouteInfo]
	updateSender   timerunner.TimeRunner
	oobPorts       syncro.Map[uint16, *OOBPacketConn]
}

const (
	connectionStateInit byte = iota
	connectionStateConnecting
	connectionStateConnected
)

// externalRouteInfo stores information about a route
type externalRouteInfo struct {
	name               string
	dest               proto.Subnet
	cost               float32
	outgoingPacketFunc func(data []byte) error
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
	lastWrite          syncro.Var[time.Time]
}

// nodeInfo represents known information about a node
type nodeInfo struct {
	name       string
	epoch      uint64
	sequence   uint64
	lastUpdate time.Time
}

// New constructs and returns a new network node on a given address
func New(ctx context.Context, addr proto.IP, name string, mtu uint16) (proto.Netopus, error) {
	if len(addr) != net.IPv6len {
		return nil, fmt.Errorf("address must be IPv6")
	}
	stack, err := netstack.NewStackDefault(ctx, net.IP(addr), mtu)
	if err != nil {
		return nil, err
	}
	n := &netopus{
		ctx:           ctx,
		addr:          addr,
		name:          name,
		mtu:           mtu,
		stack:         stack,
		router:        router.New(ctx, addr, 100*time.Millisecond),
		sessionInfo:   syncro.NewVar(make(sessInfo)),
		epoch:         uint64(time.Now().UnixNano()),
		sequence:      syncro.Var[uint64]{},
		knownNodeInfo: syncro.Map[proto.IP, nodeInfo]{},
		oobPorts:      syncro.Map[uint16, *OOBPacketConn]{},
	}
	go func() {
		routerUpdateChan := n.router.SubscribeUpdates()
		defer n.router.UnsubscribeUpdates(routerUpdateChan)
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
				log.Infof("%s: routing update: %v", n.addr.String(), strings.Join(conns, ", "))
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
			go func() {
				log.Debugf("%s: sending routing update", n.addr.String())
				ru, uerr := n.generateRoutingUpdate()
				if uerr != nil {
					log.Errorf("error generating routing update: %s", uerr)
					return
				}
				n.router.UpdateNode(n.addr, ru.Connections)
				n.floodRoutingUpdate(ru, nil)
			}()
		},
		timerunner.Periodic(10*time.Second),
		timerunner.AtStart)
	go n.monitorDeadNodes()
	return n, nil
}

// LeastMTU gives the smallest MTU of any backend defined on a node.  If there are no backends, defaultMTU is returned.
func LeastMTU(node config.Node, defaultMTU uint16) uint16 {
	mtu := uint16(math.MaxUint16)
	for _, b := range node.Backends {
		spec := backend_registry.LookupBackend(b.BackendType)
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
	for {
		data, err := p.conn.ReadMessage()
		if p.ctx.Err() != nil {
			return
		}
		if err != nil {
			if errors.Is(err, syscall.ECONNREFUSED) {
				p.cancel()
				return
			}
			log.Warnf("protocol read error: %s", err)
			continue
		}
		select {
		case <-p.ctx.Done():
			return
		case p.readChan <- data:
		}
	}
}

func (p *protoSession) routeReader() {
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
			p.cancel()
			return
		}
		viaNode := p.remoteAddr.Get()
		log.Debugf("%s: received routing update %d from %s via %s", p.n.addr.String(),
			msg.UpdateSequence, msg.Origin.String(), viaNode)
		if p.n.handleRoutingUpdate(&msg) {
			p.n.router.UpdateNode(msg.Origin, msg.Connections)
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
	p.lastWrite.Set(time.Now())
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
			log.Infof("%s: connecting to %s", p.n.addr.String(), msg.MyAddr.String())
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
			if err != nil && err != context.Canceled {
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
			return false
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
				if p.lastWrite.Get().Before(time.Now().Add(-3 * time.Second)) {
					err := p.conn.WriteMessage(ka)
					if err != nil && p.ctx.Err() == nil {
						log.Warnf("keepalive write error: %s", err)
					}
					p.lastWrite.Set(time.Now())
				}
				if lastActivity.Get().Before(time.Now().Add(-7 * time.Second)) {
					log.Warnf("%s: closing connection to %s due to inactivity", p.n.addr.String(), p.remoteAddr.Get().String())
					p.cancel()
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
		PayloadLength:     uint16(header.ICMPv6MinimumSize + extraSize),
		TransportProtocol: header.ICMPv6ProtocolNumber,
		HopLimit:          30,
		SrcAddr:           tcpip.Address(n.addr),
		DstAddr:           origPacket.SourceAddress(),
	})
	replyICMP := header.ICMPv6(replyPkt.Payload())
	replyICMP.SetType(icmpType)
	replyICMP.SetCode(icmpCode)
	copy(replyICMP[header.ICMPv6MinimumSize:], origPacket[:extraSize])
	cs := header.Checksumer{}
	cs.Add(replyICMP.Payload())
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
	dest := proto.IP(ipv6Pkt.DestinationAddress())
	if header.IsV6MulticastAddress(tcpip.Address(dest)) {
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
		return fmt.Errorf("hop limit expired")
	}
	ipv6Pkt.SetHopLimit(limit - 1)

	// Check if this packet is destined to an external route
	var opf func([]byte) error
	n.externalRoutes.WorkWithReadOnly(func(routes []externalRouteInfo) {
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
		nextHopSess.lastWrite.Set(time.Now())
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
	msg.Hops += 1
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
		return fmt.Errorf("OOB marshal error: %s", err)
	}
	go func() {
		err := nextHopSess.conn.WriteMessage(msgBytes)
		if err != nil && n.ctx.Err() == nil {
			log.Warnf("packet write error: %s", err)
		}
		nextHopSess.lastWrite.Set(time.Now())
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
		return fmt.Errorf("RouteOOB marshal error: %s", err)
	}
	go func() {
		err := peerSess.conn.WriteMessage(msgBytes)
		if err != nil && n.ctx.Err() == nil {
			log.Warnf("packet write error: %s", err)
		}
		peerSess.lastWrite.Set(time.Now())
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
	n.externalRoutes.WorkWithReadOnly(func(routes []externalRouteInfo) {
		for _, r := range routes {
			conns[r.dest] = r.cost
		}
	})
	up := &proto.RoutingUpdate{
		Origin:      n.addr,
		NodeName:    n.name,
		UpdateEpoch: n.epoch,
		Connections: conns,
	}
	n.sequence.WorkWith(func(_seq *uint64) {
		seq := *_seq
		seq += 1
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
			name:       ru.NodeName,
			epoch:      ru.UpdateEpoch,
			sequence:   ru.UpdateSequence,
			lastUpdate: time.Now(),
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
	n.externalRoutes.WorkWith(func(_routes *[]externalRouteInfo) {
		routes := *_routes
		routes = append(routes, externalRouteInfo{
			name:               name,
			dest:               dest,
			cost:               cost,
			outgoingPacketFunc: outgoingPacketFunc,
		})
		*_routes = routes
	})
}

// DelExternalRoute implements ExternalRouter
func (n *netopus) DelExternalRoute(name string) {
	n.externalRoutes.WorkWith(func(_routes *[]externalRouteInfo) {
		routes := *_routes
		newRoutes := make([]externalRouteInfo, 0, len(routes))
		for _, r := range routes {
			if r.name != name {
				newRoutes = append(newRoutes, r)
			}
		}
		*_routes = newRoutes
	})
}

// SubscribeUpdates implements ExternalRouter
func (n *netopus) SubscribeUpdates() <-chan proto.RoutingPolicy {
	return n.router.SubscribeUpdates()
}

// UnsubscribeUpdates implements ExternalRouter
func (n *netopus) UnsubscribeUpdates(ch <-chan proto.RoutingPolicy) {
	n.router.UnsubscribeUpdates(ch)
}
