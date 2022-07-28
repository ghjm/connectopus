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

const oobMTU = 1200

type OOBPacketConn struct {
	ctx          context.Context
	cancel       context.CancelFunc
	sendFunc     OOBOutboundFunc
	closeFunc    OOBCloseFunc
	sourceAddr   proto.IP
	sourcePort   uint16
	readDeadline syncro.Var[time.Time]
	inboundChan  chan *proto.OOBMessage
}

type OOBOutboundFunc func(message *proto.OOBMessage) error
type OOBCloseFunc func() error

var ErrPortInUse = fmt.Errorf("port in use")
var ErrTimeout = fmt.Errorf("timeout")
var ErrNotImplemented = fmt.Errorf("not implemented")

func (n *netopus) NewOOBPacketConn(ctx context.Context, port uint16) (net.PacketConn, error) {
	var reterr error
	pcCtx, pcCancel := context.WithCancel(ctx)
	opc := &OOBPacketConn{
		ctx:      pcCtx,
		cancel:   pcCancel,
		sendFunc: n.SendOOB,
		closeFunc: func() error {
			n.oobPorts.Delete(port)
			return nil
		},
		sourceAddr:  n.addr,
		inboundChan: make(chan *proto.OOBMessage),
	}
	n.oobPorts.WorkWith(func(_m *map[uint16]*OOBPacketConn) {
		m := *_m
		if port == 0 {
			for {
				//nolint:gosec // math/rand is ok here
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
	if reterr != nil {
		return nil, reterr
	}
	opc.sourcePort = port
	go func() {
		<-pcCtx.Done()
		_ = opc.Close()
	}()
	return opc, nil
}

func NewPacketConn(ctx context.Context, sendFunc OOBOutboundFunc, closeFunc OOBCloseFunc, sourceAddr proto.IP, sourcePort uint16) (*OOBPacketConn, error) {
	pcCtx, pcCancel := context.WithCancel(ctx)
	opc := &OOBPacketConn{
		ctx:         pcCtx,
		cancel:      pcCancel,
		sendFunc:    sendFunc,
		closeFunc:   closeFunc,
		sourceAddr:  sourceAddr,
		sourcePort:  sourcePort,
		inboundChan: make(chan *proto.OOBMessage),
	}
	go func() {
		<-pcCtx.Done()
		_ = opc.Close()
	}()
	return opc, nil
}

func (c *OOBPacketConn) IncomingPacket(msg *proto.OOBMessage) error {
	if c == nil {
		return fmt.Errorf("attempt to inject packet to nil connection")
	}
	select {
	case <-c.ctx.Done():
	case c.inboundChan <- msg:
	}
	return nil
}

func (c *OOBPacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	if c == nil {
		return 0, nil, fmt.Errorf("attempt to read from nil connection")
	}
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
		SourceAddr: c.sourceAddr,
		SourcePort: c.sourcePort,
		DestAddr:   oobAddr.Host,
		DestPort:   oobAddr.Port,
		Data:       p,
	}
	err = c.sendFunc(msg)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func (c *OOBPacketConn) Close() error {
	if c == nil {
		return fmt.Errorf("attempt to close nil connection")
	}
	if c != nil && c.cancel != nil {
		c.cancel()
	}
	if c != nil && c.closeFunc != nil {
		return c.closeFunc()
	}
	return nil
}

func (c *OOBPacketConn) LocalAddr() net.Addr {
	return proto.OOBAddr{Host: c.sourceAddr, Port: c.sourcePort}
}

func (c *OOBPacketConn) SetDeadline(t time.Time) error {
	c.readDeadline.Set(t)
	return nil
}

func (c *OOBPacketConn) SetReadDeadline(t time.Time) error {
	c.readDeadline.Set(t)
	return nil
}

func (c *OOBPacketConn) SetWriteDeadline(_ time.Time) error {
	return ErrNotImplemented
}

type OOBConn struct {
	*kcp.UDPSession
	ctx context.Context
}

func setupUDPSess(ctx context.Context, us *kcp.UDPSession) {
	us.SetMtu(oobMTU)
	us.SetNoDelay(1, 20, 2, 1)
	go func() {
		<-ctx.Done()
		_ = us.Close()
	}()
}

func DialOOB(ctx context.Context, raddr proto.OOBAddr, pc *OOBPacketConn) (net.Conn, error) {
	if pc == nil {
		return nil, fmt.Errorf("pc cannot be nil")
	}
	oc := &OOBConn{
		ctx: ctx,
	}
	var err error
	oc.UDPSession, err = kcp.NewConn2(raddr, nil, 0, 0, pc)
	if err != nil {
		return nil, err
	}
	setupUDPSess(ctx, oc.UDPSession)
	return oc, nil
}

func (n *netopus) DialOOB(ctx context.Context, raddr proto.OOBAddr) (net.Conn, error) {
	pc, err := n.NewOOBPacketConn(ctx, 0)
	if err != nil {
		return nil, err
	}
	return DialOOB(ctx, raddr, pc.(*OOBPacketConn))
}

type OOBListener struct {
	*kcp.Listener
	ctx context.Context
}

func ListenOOB(ctx context.Context, pc *OOBPacketConn) (net.Listener, error) {
	if pc == nil {
		return nil, fmt.Errorf("pc cannot be nil")
	}
	ol := &OOBListener{
		ctx: ctx,
	}
	var err error
	ol.Listener, err = kcp.ServeConn(nil, 0, 0, pc)
	if err != nil {
		return nil, err
	}
	go func() {
		<-ctx.Done()
		_ = ol.Close()
		_ = pc.Close()
	}()
	return ol, nil
}

func (n *netopus) ListenOOB(ctx context.Context, port uint16) (net.Listener, error) {
	pc, err := n.NewOOBPacketConn(ctx, port)
	if err != nil {
		return nil, err
	}
	return ListenOOB(ctx, pc.(*OOBPacketConn))
}

func (l *OOBListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	setupUDPSess(l.ctx, c.(*kcp.UDPSession))
	return c, nil
}
