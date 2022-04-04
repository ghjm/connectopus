package netopus

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/pkg/backends"
	"github.com/ghjm/connectopus/pkg/netopus/netstack"
	"github.com/ghjm/connectopus/pkg/netopus/proto"
	log "github.com/sirupsen/logrus"
	"math/rand"
	"net"
	"time"
)

// Node addressing

// Routing updates

// Data traffic

// Connections

// Known nodes

// Routing table

// Routing update subscriptions

// Netopus is the aggregate interface providing all the functionality of a netopus instance
type Netopus interface {
	backends.ProtocolRunner
}

// netopus implements Netopus
type netopus struct {
	ctx   context.Context
	addr  net.IP
	stack *netstack.NetStack
}

type protoSession struct {
	n           *netopus
	ctx         context.Context
	cancel      context.CancelFunc
	conn        backends.BackendConnection
	readChan    chan []byte
	established bool
	remoteAddr  net.IP
}

// NewNetopus constructs and returns a new network node on a given address
func NewNetopus(ctx context.Context, addr net.IP) (Netopus, error) {
	if len(addr) != net.IPv6len || !addr.IsPrivate() {
		return nil, fmt.Errorf("address must be ipv6 from the unique local range (FC00::/7)")
	}
	n := &netopus{
		ctx:   ctx,
		addr:  addr,
		stack: netstack.NewStack(ctx, addr),
	}
	return n, nil
}

// readLoop reads messages from the connection and sends them to a channel
func (p *protoSession) readLoop() {
	for {
		data, err := p.conn.ReadMessage()
		if err != nil {
			if p.ctx.Err() == nil {
				log.Warnf("protocol read error: %s", err)
				p.cancel()
			}
			return
		}
		p.readChan <- data
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
	}
}

// initLoop is the protocol run while the connection is trying to initialize
func (p *protoSession) initLoop() {
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-time.After(500 * time.Millisecond):
			p.sendInit()
		case data := <-p.readChan:
			msg, err := proto.Msg(data).Unmarshal()
			if err != nil {
				continue
			}
			switch v := msg.(type) {
			case *proto.InitMsg:
				p.remoteAddr = v.MyAddr
				return
			}
		}
	}
}

// mainLoop is the main protocol run after the connection has initialized
func (p *protoSession) mainLoop() {
	for {
		select {
		case <-p.ctx.Done():
			return
		case data := <-p.readChan:
			fmt.Printf("%s received data: %s\n", p.n.addr.String(), data)
		case <-time.After(time.Duration(500+rand.Int31n(500)) * time.Millisecond):
			_ = p.conn.WriteMessage([]byte("hello"))
		}
	}
}

// RunProtocol runs the Netopus protocol over a given backend connection
func (n *netopus) RunProtocol(ctx context.Context, conn backends.BackendConnection) {
	protoCtx, protoCancel := context.WithCancel(ctx)
	defer protoCancel()
	p := protoSession{
		n:        n,
		ctx:      protoCtx,
		cancel:   protoCancel,
		conn:     conn,
		readChan: make(chan []byte),
	}
	go p.readLoop()
	p.initLoop()
	if protoCtx.Err() != nil {
		return
	}
	fmt.Printf("%s got remote address %s\n", p.n.addr.String(), p.remoteAddr.String())
	p.mainLoop()
}
