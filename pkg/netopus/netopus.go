package netopus

import (
	"context"
	"errors"
	"fmt"
	"github.com/ghjm/connectopus/pkg/backends"
	"github.com/ghjm/connectopus/pkg/netstack"
	"github.com/ghjm/connectopus/pkg/proto"
	"github.com/ghjm/connectopus/pkg/router"
	"github.com/ghjm/connectopus/pkg/x/syncro"
	"github.com/ghjm/connectopus/pkg/x/timerunner"
	log "github.com/sirupsen/logrus"
	"golang.org/x/exp/slices"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/header"
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
	stack          netstack.NetStack
	router         router.Router
	sessionInfo    syncro.Var[sessInfo]
	epoch          uint64
	sequence       syncro.Var[uint64]
	knownNodeInfo  syncro.Map[proto.IP, nodeInfo]
	externalRoutes syncro.Var[[]externalRouteInfo]
	updateSender   timerunner.TimeRunner
}

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
	n          *netopus
	ctx        context.Context
	cancel     context.CancelFunc
	conn       backends.BackendConnection
	cost       float32
	readChan   chan proto.Msg
	remoteAddr syncro.Var[proto.IP]
	connected  syncro.Var[bool]
	connStart  time.Time
	lastInit   time.Time
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
	if mtu == 0 {
		mtu = 1400
	}
	stack, err := netstack.NewStackDefault(ctx, net.IP(addr), mtu)
	if err != nil {
		return nil, err
	}
	n := &netopus{
		ctx:           ctx,
		addr:          addr,
		name:          name,
		stack:         stack,
		router:        router.New(ctx, addr, 100*time.Millisecond),
		sessionInfo:   syncro.NewVar(make(sessInfo)),
		epoch:         uint64(time.Now().UnixNano()),
		sequence:      syncro.Var[uint64]{},
		knownNodeInfo: syncro.Map[proto.IP, nodeInfo]{},
	}
	n.sessionInfo.WorkWith(func(s *sessInfo) {
		*s = make(sessInfo)
	})
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
				log.Infof("routing update: %v", strings.Join(conns, ", "))
				n.sessionInfo.WorkWith(func(s *sessInfo) {
					for _, p := range *s {
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
				conns, upb, uerr := n.generateRoutingUpdate()
				if uerr != nil {
					log.Errorf("error generating routing update: %s", uerr)
					return
				}
				n.router.UpdateNode(n.addr, conns)
				n.flood(upb)
			}()
		},
		timerunner.Periodic(2*time.Second),
		timerunner.AtStart)
	go n.monitorDeadNodes()
	return n, nil
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

// sendInit sends an initialization message
func (p *protoSession) sendInit() {
	log.Debugf("%s: sending init message", p.n.addr.String())
	im, err := (&proto.InitMsg{MyAddr: p.n.addr}).Marshal()
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
	p.sendInit()
	timer := time.NewTimer(500 * time.Millisecond)
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
			log.Infof("%s: connected to %s", p.n.addr.String(), msg.MyAddr.String())
			p.remoteAddr.Set(msg.MyAddr)
			p.connStart = time.Now()
			p.connected.Set(true)
			p.n.sessionInfo.WorkWith(func(s *sessInfo) {
				si := *s
				oldSess, ok := si[msg.MyAddr]
				if ok {
					log.Infof("%s: closing old connection to %s", p.n.addr.String(), msg.MyAddr.String())
					oldSess.cancel()
				}
				si[msg.MyAddr] = p
			})
			p.n.updateSender.RunWithin(100 * time.Millisecond)
			return true
		default:
			log.Debugf("%s: received non-init message while in init mode", p.n.addr.String())
			return true
		}
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
		case *proto.RoutingUpdate:
			viaNode := p.remoteAddr.Get()
			log.Debugf("%s: received routing update %d from %s via %s", p.n.addr.String(),
				msg.UpdateSequence, msg.Origin.String(), viaNode)
			if p.n.handleRoutingUpdate(msg) {
				p.n.router.UpdateNode(msg.Origin, msg.Connections)
				p.n.floodExcept(data, viaNode)
			}
		case *proto.InitMsg:
			log.Debugf("%s: received init message from %s while in main loop", p.n.addr.String(),
				p.remoteAddr.Get().String())
			if time.Since(p.lastInit) > 2*time.Second {
				p.sendInit()
			}
		}
		return true
	}
}

// protoLoop is the main protocol loop
func (p *protoSession) protoLoop() {
	defer func() {
		p.cancel()
	}()
	lastActivity := syncro.Var[time.Time]{}
	go func() {
		for {
			timer := time.NewTimer(time.Second)
			select {
			case <-p.ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
				if lastActivity.Get().Before(time.Now().Add(-7 * time.Second)) {
					log.Warnf("%s: closing connection to %s due to inactivity", p.n.addr.String(), p.remoteAddr.Get().String())
					p.cancel()
				}
			}
		}
	}()
	for {
		var activity bool
		if p.connected.Get() {
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
	n.externalRoutes.WorkWithReadOnly(func(routes *[]externalRouteInfo) {
		for _, r := range *routes {
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
	n.sessionInfo.WorkWithReadOnly(func(s *sessInfo) {
		si := *s
		nextHopSess = si[nextHop]
	})
	if nextHopSess == nil || !nextHopSess.connected.Get() {
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

// floodExcept forwards a message to all neighbors except one (likely the one we received it from)
func (n *netopus) floodExcept(message proto.Msg, except proto.IP) {
	n.sessionInfo.WorkWithReadOnly(func(s *sessInfo) {
		for node, sess := range *s {
			if node == except {
				continue
			}
			if sess.connected.Get() {
				go func(sess *protoSession) {
					err := sess.conn.WriteMessage(message)
					if err != nil && n.ctx.Err() == nil {
						log.Warnf("flood error: %s", err)
					}
				}(sess)
			}
		}
	})
}

// flood forwards a message to all neighbors
func (n *netopus) flood(message proto.Msg) {
	n.floodExcept(message, "")
}

// generateRoutingUpdate produces a routing update suitable for being sent to peers.
func (n *netopus) generateRoutingUpdate() (proto.RoutingConns, []byte, error) {
	conns := make(proto.RoutingConns)
	n.sessionInfo.WorkWithReadOnly(func(s *sessInfo) {
		for _, sess := range *s {
			if sess.connected.Get() {
				conns[proto.NewSubnet(sess.remoteAddr.Get(), proto.CIDRMask(128, 128))] = sess.cost
			}
		}
	})
	n.externalRoutes.WorkWithReadOnly(func(routes *[]externalRouteInfo) {
		for _, r := range *routes {
			conns[r.dest] = r.cost
		}
	})
	up := &proto.RoutingUpdate{
		Origin:      n.addr,
		NodeName:    n.name,
		UpdateEpoch: n.epoch,
		Connections: conns,
	}
	n.sequence.WorkWith(func(seq *uint64) {
		*seq++
		up.UpdateSequence = *seq
	})
	upb, err := up.Marshal()
	if err != nil {
		return nil, nil, fmt.Errorf("error marshaling routing update: %s", err)
	}
	return conns, upb, nil
}

// handleRoutingUpdate updates the local routing table and known nodes data.  Returns a bool indicating whether the
// update should be forwarded.
func (n *netopus) handleRoutingUpdate(r *proto.RoutingUpdate) bool {
	if r.Origin.String() == n.addr.String() {
		log.Debugf("%s: rejecting routing update because we are the origin", n.addr.String())
		return false
	}
	var retval bool
	n.knownNodeInfo.WorkWith(func(_knownNodeInfo *map[proto.IP]nodeInfo) {
		knownNodeInfo := *_knownNodeInfo
		ni, ok := (knownNodeInfo)[r.Origin]
		if ok && (r.UpdateEpoch < ni.epoch || (r.UpdateEpoch == ni.epoch && r.UpdateSequence < ni.sequence)) {
			log.Debugf("%s: ignoring outdated routing update", n.addr.String())
			return
		}
		if r.UpdateEpoch == ni.epoch && r.UpdateSequence == ni.sequence {
			log.Debugf("%s: ignoring duplicate routing update", n.addr.String())
			return
		}
		retval = true
		knownNodeInfo[r.Origin] = nodeInfo{
			name:       r.NodeName,
			epoch:      r.UpdateEpoch,
			sequence:   r.UpdateSequence,
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
				if info.lastUpdate.Before(time.Now().Add(-10 * time.Second)) {
					log.Warnf("Removing dead node: %s", node.String())
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
	n.sessionInfo.WorkWithReadOnly(func(si *sessInfo) {
		for k, v := range *si {
			s.Sessions[k.String()] = proto.SessionStatus{
				Connected: v.connected.Get(),
				ConnStart: v.connStart,
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
	n.externalRoutes.WorkWith(func(routes *[]externalRouteInfo) {
		*routes = append(*routes, externalRouteInfo{
			name:               name,
			dest:               dest,
			cost:               cost,
			outgoingPacketFunc: outgoingPacketFunc,
		})
	})
}

// DelExternalRoute implements ExternalRouter
func (n *netopus) DelExternalRoute(name string) {
	n.externalRoutes.WorkWith(func(routes *[]externalRouteInfo) {
		newRoutes := make([]externalRouteInfo, 0, len(*routes))
		for _, r := range *routes {
			if r.name != name {
				newRoutes = append(newRoutes, r)
			}
		}
		*routes = newRoutes
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
