package netopus

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/pkg/proto"
	"github.com/ghjm/connectopus/pkg/x/syncro"
	"github.com/xtaci/kcp-go"
	"math/rand"
	"net"
	"time"
)

type OOBPacketConn struct {
	ctx          context.Context
	n            *netopus
	port         uint16
	readDeadline syncro.Var[time.Time]
	inboundChan  chan *proto.OOBMessage
}

var ErrPortInUse = fmt.Errorf("port in use")
var ErrTimeout = fmt.Errorf("timeout")
var ErrNotImplemented = fmt.Errorf("not implemented")

func (n *netopus) NewOOBPacketConn(ctx context.Context, port uint16) (net.PacketConn, error) {
	var reterr error
	opc := &OOBPacketConn{
		ctx:         ctx,
		n:           n,
		inboundChan: make(chan *proto.OOBMessage),
	}
	n.oobPorts.WorkWith(func(_m *map[uint16]*OOBPacketConn) {
		m := *_m
		if port == 0 {
			for {
				port = uint16(32768 + (rand.Int() % 32768))
				_, ok := m[port]
				if !ok {
					break
				}
			}
		}
		var ok bool
		_, ok = m[port]
		if ok {
			reterr = ErrPortInUse
			return
		}
		m[port] = opc
	})
	opc.port = port
	if reterr != nil {
		return nil, reterr
	}
	return opc, nil
}

func (c *OOBPacketConn) incomingPacket(msg *proto.OOBMessage) {
	select {
	case <-c.ctx.Done():
	case c.inboundChan <- msg:
	}
}

func (c *OOBPacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	readDeadline := c.readDeadline.Get()
	var timerChan <-chan time.Time
	if !readDeadline.IsZero() {
		t := time.NewTimer(time.Until(readDeadline))
		defer t.Stop()
		timerChan = t.C
	}
	select {
	case <-c.ctx.Done():
		return 0, nil, c.ctx.Err()
	case <-timerChan:
		return 0, nil, ErrTimeout
	case msg := <-c.inboundChan:
		if len(msg.Data) > len(p) {
			return 0, nil, fmt.Errorf("buffer size too small")
		}
		copy(p, msg.Data)
		return len(msg.Data), proto.OOBAddr{Host: msg.SourceAddr, Port: msg.SourcePort}, nil
	}
}

func (c *OOBPacketConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	oobAddr, ok := addr.(proto.OOBAddr)
	if !ok {
		return 0, fmt.Errorf("address not an OOB address")
	}
	msg := &proto.OOBMessage{
		SourceAddr: c.n.addr,
		SourcePort: c.port,
		DestAddr:   oobAddr.Host,
		DestPort:   oobAddr.Port,
		Data:       p,
	}
	err = c.n.SendOOB(msg)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func (c *OOBPacketConn) Close() error {
	c.n.oobPorts.Delete(c.port)
	return nil
}

func (c *OOBPacketConn) LocalAddr() net.Addr {
	return proto.OOBAddr{Host: c.n.addr, Port: c.port}
}

func (c *OOBPacketConn) SetDeadline(t time.Time) error {
	c.readDeadline.Set(t)
	return nil
}

func (c *OOBPacketConn) SetReadDeadline(t time.Time) error {
	c.readDeadline.Set(t)
	return nil
}

func (c *OOBPacketConn) SetWriteDeadline(t time.Time) error {
	return ErrNotImplemented
}

type OOBConn struct {
	*kcp.UDPSession
	ctx context.Context
	n   *netopus
	pc  net.PacketConn
}

func (n *netopus) DialOOB(ctx context.Context, raddr proto.OOBAddr) (net.Conn, error) {
	oc := &OOBConn{
		ctx: ctx,
		n:   n,
	}
	var err error
	oc.pc, err = n.NewOOBPacketConn(ctx, 0)
	if err != nil {
		return nil, err
	}
	oc.UDPSession, err = kcp.NewConn2(raddr, nil, 0, 0, oc.pc)
	if err != nil {
		return nil, err
	}
	return oc, nil
}

type OOBListener struct {
	*kcp.Listener
	ctx context.Context
	n   *netopus
	pc  net.PacketConn
}

func (n *netopus) ListenOOB(ctx context.Context, port uint16) (net.Listener, error) {
	ol := &OOBListener{
		ctx: ctx,
		n:   n,
	}
	var err error
	ol.pc, err = n.NewOOBPacketConn(ctx, port)
	if err != nil {
		return nil, err
	}
	ol.Listener, err = kcp.ServeConn(nil, 0, 0, ol.pc)
	if err != nil {
		return nil, err
	}
	return ol, nil
}
