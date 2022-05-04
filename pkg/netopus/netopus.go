package netopus

import (
	"context"
	"errors"
	"fmt"
	"github.com/ghjm/connectopus/pkg/backends"
	"github.com/ghjm/connectopus/pkg/netopus/netstack"
	"github.com/ghjm/connectopus/pkg/netopus/proto"
	"github.com/ghjm/connectopus/pkg/netopus/router"
	"github.com/ghjm/connectopus/pkg/utils/syncro"
	log "github.com/sirupsen/logrus"
	"golang.org/x/exp/slices"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"math/rand"
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
}

// netopus implements Netopus
type netopus struct {
	ctx               context.Context
	addr              net.IP
	stack             netstack.NetStack
	router            router.Router[string]
	sessionInfo       syncro.Var[sessInfo]
	epoch             uint64
	sequence          syncro.Var[uint64]
	seenUpdates       syncro.Map[uint64, time.Time]
	knownNodeInfo     syncro.Map[string, nodeInfo]
	lastRoutingUpdate syncro.Var[time.Time]
	externalRoutes    syncro.Var[[]externalRouteInfo]
}

// externalRouteInfo stores information about a route
type externalRouteInfo struct {
	dest               net.IP
	outgoingPacketFunc func(data []byte) error
}

// sessInfo stores information about sessions
type sessInfo struct {
	sessions     map[uint64]*protoSession
	nodeSessions map[string]uint64
}

// protoSession represents one running session of a protocol
type protoSession struct {
	n          *netopus
	id         uint64
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
	stack, err := netstack.NewStack(ctx, subnet, addr)
	if err != nil {
		return nil, err
	}
	n := &netopus{
		ctx:               ctx,
		addr:              addr,
		stack:             stack,
		router:            router.New(ctx, addr.String(), 50*time.Millisecond),
		sessionInfo:       syncro.Var[sessInfo]{},
		epoch:             uint64(time.Now().UnixNano()),
		sequence:          syncro.Var[uint64]{},
		seenUpdates:       syncro.Map[uint64, time.Time]{},
		knownNodeInfo:     syncro.Map[string, nodeInfo]{},
		lastRoutingUpdate: syncro.Var[time.Time]{},
	}
	n.sessionInfo.WorkWith(func(s *sessInfo) {
		s.sessions = make(map[uint64]*protoSession)
		s.nodeSessions = make(map[string]uint64)
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
			}
		}
	}()
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

// netStackReadLoop reads packets from the netstack and processes them
func (p *protoSession) netStackReadLoop() {
	readChan := p.n.stack.SubscribePackets()
	for {
		select {
		case <-p.ctx.Done():
			return
		case packet := <-readChan:
			err := p.n.SendPacket(packet)
			if p.ctx.Err() != nil {
				return
			}
			if err != nil {
				log.Warnf("protocol write error: %s", err)
				continue
			}
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
	select {
	case <-p.ctx.Done():
		return false
	case <-time.After(500 * time.Millisecond):
		p.sendInit()
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
				s.nodeSessions[msg.MyAddr.String()] = p.id
			})
			p.n.sendRoutingUpdate()
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
	case <-time.After(time.Second):
		log.Debugf("%s: sending routing update", p.n.addr.String())
		p.n.sendRoutingUpdate()
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
			if err != nil {
				log.Warnf("packet sending error: %s", err)
			}
		case *proto.RoutingUpdate:
			log.Debugf("%s: received routing update %d from %s via %s", p.n.addr.String(),
				msg.UpdateSequence, msg.Origin.String(), p.remoteAddr.Get().String())
			if p.n.handleRoutingUpdate(msg) {
				p.n.router.UpdateNode(msg.Origin.String(), msg.Connections.GetConnMap())
				p.n.flood(data)
				p.n.rateLimitedSendRoutingUpdate()
			}
		case *proto.InitMsg:
			log.Debugf("%s: received init message from %s while in main loop", p.n.addr.String(),
				p.remoteAddr.Get().String())
			if time.Now().Sub(p.lastInit) > time.Second {
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
			select {
			case <-p.ctx.Done():
				return
			case <-time.After(time.Second):
				if lastActivity.Get().Before(time.Now().Add(-5 * time.Second)) {
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
	n.sessionInfo.WorkWith(func(s *sessInfo) {
		for {
			p.id = rand.Uint64()
			_, ok := s.sessions[p.id]
			if ok {
				continue
			}
			s.sessions[p.id] = p
			break
		}
	})
	defer func() {
		n.sessionInfo.WorkWith(func(s *sessInfo) {
			delete(s.sessions, p.id)
			for k, v := range s.nodeSessions {
				if v == p.id {
					delete(s.nodeSessions, k)
				}
			}
		})
		n.sendRoutingUpdate()
	}()
	go p.backendReadLoop()
	go p.netStackReadLoop()
	go p.protoLoop()
	<-protoCtx.Done()
}

// SendPacket sends a single IPv6 packet over the network.
func (n *netopus) SendPacket(packet []byte) error {
	if len(packet) < header.IPv6MinimumSize {
		return fmt.Errorf("malformed packet: too small")
	}
	dest := header.IPv6(packet).DestinationAddress()
	if dest == tcpip.Address(n.addr) {
		return n.stack.SendPacket(packet)
	}
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
	nextHop := n.router.NextHop(dest.String())
	if nextHop == "" {
		log.Debugf("%s: trying to recover from no route to host", n.addr.String())
		n.sendRoutingUpdate()
		time.Sleep(500 * time.Millisecond)
		nextHop = n.router.NextHop(dest.String())
	}
	if nextHop == "" {
		return fmt.Errorf("no route to host")
	}
	var nextHopSess *protoSession
	n.sessionInfo.WorkWithReadOnly(func(s *sessInfo) {
		id, ok := s.nodeSessions[nextHop]
		if ok {
			nextHopSess = s.sessions[id]
		}
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

// flood forwards a message to all neighbors
func (n *netopus) flood(message proto.Msg) {
	n.sessionInfo.WorkWithReadOnly(func(s *sessInfo) {
		for _, sess := range s.sessions {
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

// generateRoutingUpdate produces a routing update suitable for being sent to peers.
func (n *netopus) generateRoutingUpdate() (proto.RoutingConnections, []byte, error) {
	conns := make(proto.RoutingConnections, 0)
	n.sessionInfo.WorkWithReadOnly(func(s *sessInfo) {
		for _, sess := range s.sessions {
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
		UpdateID:    rand.Uint64(),
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

// sendRoutingUpdate sends a routing update to all neighbors
func (n *netopus) sendRoutingUpdate() {
	conns, upb, err := n.generateRoutingUpdate()
	if err != nil {
		log.Errorf("error generating routing update: %s", err)
		return
	}
	n.lastRoutingUpdate.Set(time.Now())
	n.router.UpdateNode(n.addr.String(), conns.GetConnMap())
	n.flood(upb)
}

// rateLimitedSendRoutingUpdate sends a routing update, only if one has not already been sent recently
func (n *netopus) rateLimitedSendRoutingUpdate() {
	if time.Now().Sub(n.lastRoutingUpdate.Get()) > time.Second {
		n.sendRoutingUpdate()
	}
}

// handleRoutingUpdate updates the local routing table and known nodes data.  Returns a bool indicating whether the
// update should be forwarded.
func (n *netopus) handleRoutingUpdate(r *proto.RoutingUpdate) bool {
	if r.Origin.String() == n.addr.String() {
		log.Debugf("%s: rejecting routing update because we are the origin", n.addr.String())
		return false
	}
	_, seen := n.seenUpdates.Get(r.UpdateID)
	n.seenUpdates.Set(r.UpdateID, time.Now())
	if seen {
		log.Debugf("%s: rejecting routing update because it was already seen", n.addr.String())
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
			log.Debugf("%s: ignoring unchanged routing update", n.addr.String())
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
