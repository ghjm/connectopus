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

// Netopus is the aggregate interface providing all the functionality of a netopus instance
type Netopus interface {
	backends.ProtocolRunner
	netstack.UserStack
	ExternalRouter
}

// ExternalRouter is a device that can accept and send packets to external routes
type ExternalRouter interface {
	// AddExternalRoute adds an external route.  When packets arrive for this destination, outgoingPacketFunc will be called.
	AddExternalRoute(net.IP, func([]byte) error)
	// DelExternalRoute removes a previously added external route.  If the route does not exist, this has no effect.
	DelExternalRoute(net.IP)
	// SendPacket routes and sends a packet
	SendPacket(packet []byte) error
}

// netopus implements Netopus
type netopus struct {
	ctx            context.Context
	addr           net.IP
	stack          netstack.NetStack
	router         router.Router[string]
	sessionInfo    syncro.Var[sessInfo]
	epoch          uint64
	sequence       syncro.Var[uint64]
	knownNodeInfo  syncro.Map[string, nodeInfo]
	externalRoutes syncro.Var[[]externalRouteInfo]
	updateSender   timerunner.TimeRunner
}

// externalRouteInfo stores information about a route
type externalRouteInfo struct {
	dest               net.IP
	outgoingPacketFunc func(data []byte) error
}

// sessInfo stores information about sessions
type sessInfo map[string]*protoSession

// protoSession represents one running session of a protocol
type protoSession struct {
	n          *netopus
	ctx        context.Context
	cancel     context.CancelFunc
	conn       backends.BackendConnection
	readChan   chan proto.Msg
	remoteAddr syncro.Var[net.IP]
	connected  syncro.Var[bool]
	connStart  time.Time
	lastInit   time.Time
}

// nodeInfo represents known information about a node
type nodeInfo struct {
	epoch    uint64
	sequence uint64
}

// NewNetopus constructs and returns a new network node on a given address
func NewNetopus(ctx context.Context, subnet *net.IPNet, addr net.IP) (Netopus, error) {
	if len(addr) != net.IPv6len || len(subnet.IP) != net.IPv6len {
		return nil, fmt.Errorf("subnet and address must be IPv6")
	}
	stack, err := netstack.NewStackDefault(ctx, subnet, addr)
	if err != nil {
		return nil, err
	}
	n := &netopus{
		ctx:           ctx,
		addr:          addr,
		stack:         stack,
		router:        router.New(ctx, addr.String(), 100*time.Millisecond),
		sessionInfo:   syncro.NewVar(make(sessInfo)),
		epoch:         uint64(time.Now().UnixNano()),
		sequence:      syncro.Var[uint64]{},
		knownNodeInfo: syncro.Map[string, nodeInfo]{},
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
				n.router.UpdateNode(n.addr.String(), conns.GetConnMap())
				n.flood(upb)
			}()
		},
		timerunner.Periodic(2*time.Second),
		timerunner.AtStart)
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
				oldSess, ok := si[msg.MyAddr.String()]
				if ok {
					log.Infof("%s: closing old connection to %s", p.n.addr.String(), msg.MyAddr.String())
					oldSess.cancel()
				}
				si[msg.MyAddr.String()] = p
			})
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
			viaNode := p.remoteAddr.Get().String()
			log.Debugf("%s: received routing update %d from %s via %s", p.n.addr.String(),
				msg.UpdateSequence, msg.Origin.String(), viaNode)
			if p.n.handleRoutingUpdate(msg) {
				p.n.router.UpdateNode(msg.Origin.String(), msg.Connections.GetConnMap())
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
func (n *netopus) RunProtocol(ctx context.Context, conn backends.BackendConnection) {
	protoCtx, protoCancel := context.WithCancel(ctx)
	defer func() {
		protoCancel()
	}()
	p := &protoSession{
		n:        n,
		ctx:      protoCtx,
		cancel:   protoCancel,
		conn:     conn,
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
	dest := ipv6Pkt.DestinationAddress()
	if header.IsV6MulticastAddress(dest) {
		return nil
	}

	// Check if we can deliver the packet locally
	if dest == tcpip.Address(n.addr) {
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
			if dest == tcpip.Address(r.dest) {
				opf = r.outgoingPacketFunc
				break
			}
		}
	})
	if opf != nil {
		return opf(packet)
	}

	// Send the packet to the next hop
	nextHop := n.router.NextHop(dest.String())
	if nextHop == "" {
		log.Debugf("%s: trying to recover from no route to host", n.addr.String())
		n.updateSender.RunWithin(100 * time.Millisecond)
		time.Sleep(500 * time.Millisecond)
		nextHop = n.router.NextHop(dest.String())
	}
	if nextHop == "" {
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
func (n *netopus) floodExcept(message proto.Msg, except string) {
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
func (n *netopus) generateRoutingUpdate() (proto.RoutingConnections, []byte, error) {
	conns := make(proto.RoutingConnections, 0)
	n.sessionInfo.WorkWithReadOnly(func(s *sessInfo) {
		for _, sess := range *s {
			if sess.connected.Get() {
				conns = append(conns, proto.RoutingConnection{
					Peer: sess.remoteAddr.Get(),
					Cost: 1.0,
				})
			}
		}
	})
	n.externalRoutes.WorkWithReadOnly(func(routes *[]externalRouteInfo) {
		for _, r := range *routes {
			conns = append(conns, proto.RoutingConnection{
				Peer: r.dest,
				Cost: 1.0,
			})
		}
	})
	up := &proto.RoutingUpdate{
		Origin:      n.addr,
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
	n.knownNodeInfo.WorkWith(func(knownNodeInfo *map[string]nodeInfo) {
		origin := r.Origin.String()
		ni, ok := (*knownNodeInfo)[origin]
		if ok && (r.UpdateEpoch < ni.epoch || (r.UpdateEpoch == ni.epoch && r.UpdateSequence < ni.sequence)) {
			log.Debugf("%s: ignoring outdated routing update", n.addr.String())
			return
		}
		if r.UpdateEpoch == ni.epoch && r.UpdateSequence == ni.sequence {
			log.Debugf("%s: ignoring duplicate routing update", n.addr.String())
			return
		}
		retval = true
		(*knownNodeInfo)[origin] = nodeInfo{
			epoch:    r.UpdateEpoch,
			sequence: r.UpdateSequence,
		}
	})
	return retval
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
func (n *netopus) AddExternalRoute(dest net.IP, outgoingPacketFunc func([]byte) error) {
	n.externalRoutes.WorkWith(func(routes *[]externalRouteInfo) {
		*routes = append(*routes, externalRouteInfo{
			dest:               dest,
			outgoingPacketFunc: outgoingPacketFunc,
		})
	})
}

// DelExternalRoute implements ExternalRouter
func (n *netopus) DelExternalRoute(dest net.IP) {
	n.externalRoutes.WorkWith(func(routes *[]externalRouteInfo) {
		newRoutes := make([]externalRouteInfo, 0, len(*routes))
		for _, r := range *routes {
			if string(r.dest) != string(dest) {
				newRoutes = append(newRoutes, r)
			}
		}
		*routes = newRoutes
	})
}
