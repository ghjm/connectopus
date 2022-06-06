package proto

import (
	"encoding/json"
	"fmt"
	"net"
)

// Data structures for the Netopus wire protocol

// Msg is the type of a single Netopus protocol datagram
type Msg []byte

// MsgType enumerates the types of Netopus protocol messages
type MsgType int

const (
	MsgTypeError MsgType = -1
	MsgTypeData  MsgType = 0
	MsgTypeInit  MsgType = 1
	MsgTypeRoute MsgType = 2
	MaxMsgType   MsgType = 2
)

// InitMsg is a message type sent at connection initialization time
type InitMsg struct {
	MyAddr net.IP
}

// RoutingUpdate is a message type carrying routing information
type RoutingUpdate struct {
	Origin         net.IP
	UpdateEpoch    uint64
	UpdateSequence uint64
	Connections    RoutingConnections
}

// RoutingConnection is the information of a single connection in a routing update
type RoutingConnection struct {
	Peer net.IP
	Cost float32
}

// RoutingConnections is a slice of RoutingConnection
type RoutingConnections []RoutingConnection

var ErrUnknownMessageType = fmt.Errorf("unknown message type")

// Type returns the MsgType of a Msg
func (m Msg) Type() MsgType {
	if len(m) < 1 {
		return MsgTypeError
	}
	b := m[0]
	switch {
	case b>>4 == 6: // Data packets are just unmodified IPv6 packets
		return MsgTypeData
	case MsgType(b) <= MaxMsgType:
		return MsgType(b)
	default:
		return MsgTypeError
	}
}

// Unmarshal unmarshals a message, returning []byte for a data message or a pointer to struct for other types
func (m Msg) Unmarshal() (any, error) {
	unmarshalMsg := func(v any) (any, error) {
		err := json.Unmarshal(m[1:], v)
		if err != nil {
			return nil, err
		}
		return v, nil
	}
	switch m.Type() {
	case MsgTypeData:
		return []byte(m), nil
	case MsgTypeInit:
		return unmarshalMsg(&InitMsg{})
	case MsgTypeRoute:
		return unmarshalMsg(&RoutingUpdate{})
	}
	return nil, ErrUnknownMessageType
}

// marshalMsg marshals a message of some type to a []byte
func marshalMsg[T any](m T, msgType MsgType) ([]byte, error) {
	d, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	d = append([]byte{byte(msgType)}, d...)
	return d, nil
}

// Marshal marshals an InitMsg to a []byte
func (m *InitMsg) Marshal() ([]byte, error) {
	d, err := marshalMsg(m, MsgTypeInit)
	if err != nil {
		return nil, err
	}
	return d, nil
}

// Marshal marshals a RoutingUpdate to a []byte
func (ru *RoutingUpdate) Marshal() ([]byte, error) {
	d, err := marshalMsg(ru, MsgTypeRoute)
	if err != nil {
		return nil, err
	}
	return d, nil
}

func (ru *RoutingUpdate) String() string {
	return fmt.Sprintf("%s/%d/%d", ru.Origin.String(), ru.UpdateEpoch, ru.UpdateSequence)
}

// GetConnMap gets a map of connection costs, suitable for use in router.UpdateNode
func (rc RoutingConnections) GetConnMap() map[string]float32 {
	m := make(map[string]float32)
	for _, c := range rc {
		m[c.Peer.String()] = 1.0
	}
	return m
}
