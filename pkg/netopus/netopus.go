package netopus

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/pkg/backends"
	"github.com/ghjm/connectopus/pkg/netopus/netstack"
	"github.com/ghjm/connectopus/pkg/netopus/proto"
	"github.com/ghjm/connectopus/pkg/netopus/router"
	"github.com/ghjm/connectopus/pkg/utils/syncromap"
	"github.com/ghjm/connectopus/pkg/utils/syncrovar"
	log "github.com/sirupsen/logrus"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"math/rand"
	"net"
	"time"
)

// Netopus is the aggregate interface providing all the functionality of a netopus instance
type Netopus interface {
	backends.ProtocolRunner
	Gonet
}

// Gonet is the Go net style interface for Go modules to use
type Gonet interface {
	DialTCP(net.IP, uint16) (net.Conn, error)
	DialContextTCP(context.Context, net.IP, uint16) (net.Conn, error)
	ListenTCP(uint16) (net.Listener, error)
	DialUDP(uint16, net.IP, uint16) (UDPConn, error)
}

// UDPConn is the netopus equivalent to net.UDPConn
type UDPConn interface {
	net.Conn
	net.PacketConn
}

// netopus implements Netopus
type netopus struct {
	ctx           context.Context
	addr          net.IP
	stack         *netstack.NetStack
	router        router.Router[string]
	sessionInfo   syncrovar.SyncroVar[sessInfo]
	epoch         uint64
	sequence      syncrovar.SyncroVar[uint64]
	seenUpdates   syncromap.SyncroMap[uint64, time.Time]
	knownNodeInfo syncromap.SyncroMap[string, nodeInfo]
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
	remoteAddr net.IP
	connected  syncrovar.SyncroVar[bool]
	connStart  time.Time
}

// nodeInfo represents known information about a node
type nodeInfo struct {
	epoch    uint64
	sequence uint64
}

// NewNetopus constructs and returns a new network node on a given address
func NewNetopus(ctx context.Context, addr net.IP) (Netopus, error) {
	if len(addr) != net.IPv6len || !addr.IsPrivate() {
		return nil, fmt.Errorf("address must be ipv6 from the unique local range (FC00::/7)")
	}
	stack, err := netstack.NewStack(ctx, addr)
	if err != nil {
		return nil, err
	}
	n := &netopus{
		ctx:           ctx,
		addr:          addr,
		stack:         stack,
		router:        router.New(ctx, addr.String(), 50*time.Millisecond),
		sessionInfo:   syncrovar.SyncroVar[sessInfo]{},
		epoch:         uint64(time.Now().UnixNano()),
		sequence:      syncrovar.SyncroVar[uint64]{},
		seenUpdates:   syncromap.NewMap[uint64, time.Time](),
		knownNodeInfo: syncromap.NewMap[string, nodeInfo](),
	}
	n.sessionInfo.WorkWith(func(s *sessInfo) {
		s.sessions = make(map[uint64]*protoSession)
		s.nodeSessions = make(map[string]uint64)
	})
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
			log.Warnf("protocol read error: %s", err)
		}
		select {
		case <-p.ctx.Done():
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
			if err != nil {
				if p.ctx.Err() != nil {
					return
				}
				log.Warnf("protocol write error: %s", err)
			}
		}
	}
}

// sendInit sends an initialization message
func (p *protoSession) sendInit() {
	im, err := (&proto.InitMsg{MyAddr: p.n.addr}).Marshal()
	if err == nil {
		err = p.conn.WriteMessage(im)
	}
	if err != nil {
		log.Warnf("error sending init message: %s", err)
		return
	}
}

// initSelect runs one instance of the main loop when the connection is not established
func (p *protoSession) initSelect() {
	p.sendInit()
	select {
	case <-p.ctx.Done():
		return
	case <-time.After(500 * time.Millisecond):
		p.sendInit()
	case data := <-p.readChan:
		msg, err := data.Unmarshal()
		if err != nil {
			return
		}
		switch v := msg.(type) {
		case *proto.InitMsg:
			p.remoteAddr = v.MyAddr
			p.connStart = time.Now()
			p.connected.Set(true)
			p.n.sessionInfo.WorkWith(func(s *sessInfo) {
				s.nodeSessions[p.remoteAddr.String()] = p.id
			})
			p.n.sendRoutingUpdate()
			return
		}
	}
}

// mainSelect runs one instance of the main loop when the connection is established
func (p *protoSession) mainSelect() {
	select {
	case <-p.ctx.Done():
		return
	case <-time.After(time.Second):
		p.n.sendRoutingUpdate()
	case data := <-p.readChan:
		msgAny, err := data.Unmarshal()
		if err != nil {
			log.Warnf("packet unmarshaling error: %s", err)
			return
		}
		switch msg := msgAny.(type) {
		case []byte:
			err = p.n.SendPacket(data)
			if err != nil {
				log.Warnf("packet sending error: %s", err)
				return
			}
		case *proto.RoutingUpdate:
			if p.n.handleRoutingUpdate(msg) {
				p.n.flood(data, &p.remoteAddr)
			}
		case *proto.InitMsg:
			if time.Now().Sub(p.connStart) > 2*time.Second {
				// The remote side doesn't think we're connected, so re-run the init process
				p.connected.Set(false)
				p.n.router.RemoveNode(p.remoteAddr.String())
			}
		}
	}
}

// protoLoop is the main protocol loop
func (p *protoSession) protoLoop() {
	defer func() {
		p.cancel()
	}()
	for {
		if p.connected.Get() {
			p.mainSelect()
		} else {
			p.initSelect()
		}
		if p.ctx.Err() != nil {
			return
		}
	}
}

// RunProtocol runs the Netopus protocol over a given backend connection
func (n *netopus) RunProtocol(ctx context.Context, conn backends.BackendConnection) {
	protoCtx, protoCancel := context.WithCancel(ctx)
	defer protoCancel()
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
	defer n.sessionInfo.WorkWith(func(s *sessInfo) {
		delete(s.sessions, p.id)
		for k, v := range s.nodeSessions {
			if v == p.id {
				delete(s.nodeSessions, k)
			}
		}
	})
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
	nextHop := n.router.NextHop(dest.String())
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

// flood forwards a message to all neighbors, possibly excluding one.
func (n *netopus) flood(message proto.Msg, excludeConn *net.IP) {
	n.sessionInfo.WorkWithReadOnly(func(s *sessInfo) {
		for _, sess := range s.sessions {
			if excludeConn == nil || !excludeConn.Equal(sess.remoteAddr) {
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

// sendRoutingUpdate updates the local node in the router, and generates and sends a routing update to peers.
func (n *netopus) sendRoutingUpdate() {
	conns := make(proto.RoutingConnections, 0)
	n.sessionInfo.WorkWithReadOnly(func(s *sessInfo) {
		for _, sess := range s.sessions {
			if sess.connected.Get() {
				conns = append(conns, proto.RoutingConnection{
					Peer: sess.remoteAddr,
					Cost: 1.0,
				})
			}
		}
	})
	n.router.UpdateNode(n.addr.String(), conns.GetConnMap())
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
		log.Warnf("error marshaling routing update: %s", err)
		return
	}
	n.flood(upb, nil)
}

// handleRoutingUpdate updates the local routing table and known nodes data.  Returns a bool indicating whether the
// update should be forwarded.
func (n *netopus) handleRoutingUpdate(r *proto.RoutingUpdate) bool {
	var seen bool
	func() {
		tr := n.seenUpdates.BeginTransaction()
		defer tr.EndTransaction()
		_, seen = tr.Get(r.UpdateID)
		tr.Set(r.UpdateID, time.Now())
	}()
	if seen {
		return false
	}
	accepted := false
	func() {
		tr := n.knownNodeInfo.BeginTransaction()
		defer tr.EndTransaction()
		ni, ok := tr.Get(string(r.Origin))
		if ok && (r.UpdateEpoch < ni.epoch || (r.UpdateEpoch == ni.epoch && r.UpdateSequence < ni.sequence)) {
			// This is an out-of-date update, so ignore it
			return
		}
		accepted = true
		tr.Set(string(r.Origin), nodeInfo{
			epoch:    r.UpdateEpoch,
			sequence: r.UpdateSequence,
		})
	}()
	if !accepted {
		return false
	}
	//TODO: detect unchanged connections and avoid routing update in that case
	n.router.UpdateNode(r.Origin.String(), r.Connections.GetConnMap())
	return true
}
