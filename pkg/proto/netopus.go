package proto

import (
	"github.com/ghjm/connectopus/pkg/backends"
	"github.com/ghjm/connectopus/pkg/netstack"
	"time"
)

// Netopus is the aggregate interface providing all the functionality of a netopus instance
type Netopus interface {
	backends.ProtocolRunner
	netstack.UserStack
	ExternalRouter
	StatusGetter
}

// ExternalRouter is a device that can accept and send packets to external routes
type ExternalRouter interface {
	// AddExternalRoute adds an external route.  When packets arrive for this destination, outgoingPacketFunc will be called.
	AddExternalRoute(string, Subnet, float32, func([]byte) error)
	// DelExternalRoute removes a previously added external route.  If the route does not exist, this has no effect.
	DelExternalRoute(string)
	// SendPacket routes and sends a packet
	SendPacket(packet []byte) error
}

type StatusGetter interface {
	Status() *Status
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
