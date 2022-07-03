package proto

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/pkg/backends"
	"github.com/ghjm/connectopus/pkg/netstack"
	"net"
	"time"
)

// Netopus is the aggregate interface providing all the functionality of a netopus instance
type Netopus interface {
	backends.ProtocolRunner
	netstack.UserStack
	ExternalRouter
	StatusGetter
	OOBConnector
}

// ExternalRouter is a device that can accept and send packets to external routes
type ExternalRouter interface {
	// AddExternalRoute adds an external route.  When packets arrive for this destination, outgoingPacketFunc will be called.
	AddExternalRoute(string, Subnet, float32, func([]byte) error)
	// DelExternalRoute removes a previously added external route.  If the route does not exist, this has no effect.
	DelExternalRoute(string)
	// SendPacket routes and sends a packet
	SendPacket(packet []byte) error
	// SubscribeUpdates returns a channel that will be sent to whenever the routing policy changes
	SubscribeUpdates() <-chan RoutingPolicy
	// UnsubscribeUpdates unsubscribes a previously subscribed updates channel
	UnsubscribeUpdates(<-chan RoutingPolicy)
}

type StatusGetter interface {
	Status() *Status
}

type OOBAddr struct {
	Host IP
	Port uint16
}

func (a OOBAddr) Network() string {
	return "oob"
}

func (a OOBAddr) String() string {
	return fmt.Sprintf("%s:%d", a.Host, a.Port)
}

type OOBConnector interface {
	NewOOBPacketConn(ctx context.Context, port uint16) (net.PacketConn, error)
	DialOOB(ctx context.Context, raddr OOBAddr) (net.Conn, error)
	ListenOOB(ctx context.Context, port uint16) (net.Listener, error)
}

// Status is returned by netopus.Status()
type Status struct {
	Name        string
	Addr        IP
	NameToAddr  map[string]string
	AddrToName  map[string]string
	RouterNodes map[string]map[string]float32
	Sessions    map[string]SessionStatus
}

// SessionStatus represents the status of a single session
type SessionStatus struct {
	Connected bool
	ConnStart time.Time
}
