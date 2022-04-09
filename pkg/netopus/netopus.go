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
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv6"
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

// UDPConn works the same as net.UDPConn
type UDPConn interface {
	net.Conn
	net.PacketConn
}

// netopus implements Netopus
type netopus struct {
	ctx      context.Context
	addr     net.IP
	stack    *netstack.NetStack
	router   router.Router
	sessions syncromap.SyncroMap[int, *protoSession]
}

// protoSession represents one running session of a protocol
type protoSession struct {
	n          *netopus
	ctx        context.Context
	cancel     context.CancelFunc
	conn       backends.BackendConnection
	readChan   chan proto.Msg
	remoteAddr net.IP
	connected  syncrovar.SyncroVar[bool]
	connStart  time.Time
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
		ctx:      ctx,
		addr:     addr,
		stack:    stack,
		router:   router.New(ctx, string(addr), 50*time.Millisecond),
		sessions: syncromap.NewMap[int, *protoSession](),
	}
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
			return
		}
	}
}

// mainSelect runs one instance of the main loop when the connection is established
func (p *protoSession) mainSelect() {
	select {
	case <-p.ctx.Done():
		return
	case data := <-p.readChan:
		msg, err := data.Unmarshal()
		if err != nil {
			log.Warnf("packet unmarshaling error: %s", err)
			return
		}
		switch msg.(type) {
		case []byte:
			err = p.n.SendPacket(data)
			if err != nil {
				log.Warnf("packet sending error: %s", err)
				return
			}
		case proto.RoutingUpdate:
		case proto.InitMsg:
			if time.Now().Sub(p.connStart) > 2*time.Second {
				// The remote side doesn't think we're connected, so re-run the init process
				p.connected.Set(false)
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
	p := protoSession{
		n:         n,
		ctx:       protoCtx,
		cancel:    protoCancel,
		conn:      conn,
		readChan:  make(chan proto.Msg),
	}
	var sessID int
	for {
		sessID = rand.Int()
		err := n.sessions.Create(sessID, &p)
		if err == nil {
			break
		}
	}
	defer n.sessions.Delete(sessID)
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
	} else {
		var nextHopConn *protoSession
		func() {
			st := n.sessions.BeginTransaction()
			defer st.EndTransaction()
			for _, conn := range *st.RawMap() {
				if conn.connected.Get() && tcpip.Address(conn.remoteAddr) == dest {
					nextHopConn = conn
					break
				}
			}
		}()
		if nextHopConn != nil {
			go func() {
				err := nextHopConn.conn.WriteMessage(packet)
				if err != nil && n.ctx.Err() == nil {
					log.Warnf("packet write error: %s", err)
				}
			}()
			return nil
		}
	}
	return nil
}

// DialTCP dials a TCP connection over the Netopus network
func (n *netopus) DialTCP(addr net.IP, port uint16) (net.Conn, error) {
	return gonet.DialTCP(
		n.stack.Stack,
		tcpip.FullAddress{
			NIC:  1,
			Addr: tcpip.Address(addr),
			Port: port,
		},
		ipv6.ProtocolNumber)
}

// DialContextTCP dials a TCP connection over the Netopus network, using a context
func (n *netopus) DialContextTCP(ctx context.Context, addr net.IP, port uint16) (net.Conn, error) {
	return gonet.DialContextTCP(ctx,
		n.stack.Stack,
		tcpip.FullAddress{
			NIC:  1,
			Addr: tcpip.Address(addr),
			Port: port,
		},
		ipv6.ProtocolNumber)
}

// ListenTCP opens a TCP listener over the Netopus network
func (n *netopus) ListenTCP(port uint16) (net.Listener, error) {
	return gonet.ListenTCP(
		n.stack.Stack,
		tcpip.FullAddress{
			NIC:  1,
			Addr: tcpip.Address(n.addr),
			Port: port,
		},
		ipv6.ProtocolNumber)
}

// DialUDP opens a UDP sender or receiver over the Netopus network
func (n *netopus) DialUDP(lport uint16, addr net.IP, rport uint16) (UDPConn, error) {
	var laddr *tcpip.FullAddress
	if lport != 0 {
		laddr = &tcpip.FullAddress{
			NIC:  1,
			Addr: tcpip.Address(n.addr),
			Port: lport,
		}
	}
	var raddr *tcpip.FullAddress
	if addr != nil {
		raddr = &tcpip.FullAddress{
			NIC:  1,
			Addr: tcpip.Address(addr),
			Port: rport,
		}
	}
	return gonet.DialUDP(n.stack.Stack, laddr, raddr, ipv6.ProtocolNumber)
}
