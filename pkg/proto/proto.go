package proto

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
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
	MsgTypeOOB   MsgType = 3
	MaxMsgType   MsgType = 3
)

// InitMsg is a message type sent at connection initialization time
type InitMsg struct {
	MyAddr IP
}

// RoutingNodes stores the known connection information for a network of nodes
type RoutingNodes map[IP]RoutingConns

// RoutingConns stores the known connections and costs directly connected to a node
type RoutingConns map[Subnet]float32

// RoutingPolicy is a routing table giving the next hop for a list of subnets
type RoutingPolicy map[Subnet]IP

// RoutingUpdate is a message type carrying routing information
type RoutingUpdate struct {
	Origin         IP
	NodeName       string
	UpdateEpoch    uint64
	UpdateSequence uint64
	Connections    RoutingConns
}

// OOBMessage is an out-of-band message
type OOBMessage struct {
	Hops       byte
	SourceAddr IP
	SourcePort uint16
	DestAddr   IP
	DestPort   uint16
	Data       []byte
}

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
	case MsgTypeOOB:
		return m.unmarshalOOB()
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

const oobHeaderHops = 1
const oobHeaderSourceAddr = 2
const oobHeaderSourcePort = 18
const oobHeaderDestAddr = 20
const oobHeaderDestPort = 37
const oobHeaderData = 39

// Marshal marshals an OOBMessage to a []byte
func (oob *OOBMessage) Marshal() ([]byte, error) {
	b := make([]byte, oobHeaderData+len(oob.Data))
	b[0] = byte(MsgTypeOOB)
	b[oobHeaderHops] = oob.Hops
	copy(b[oobHeaderSourceAddr:oobHeaderSourceAddr+16], oob.SourceAddr)
	binary.BigEndian.PutUint16(b[oobHeaderSourcePort:oobHeaderSourcePort+2], oob.SourcePort)
	copy(b[oobHeaderDestAddr:oobHeaderDestAddr+16], oob.DestAddr)
	binary.BigEndian.PutUint16(b[oobHeaderDestPort:oobHeaderDestPort+2], oob.DestPort)
	copy(b[oobHeaderData:], oob.Data)
	return b, nil
}

// String produces a string representation of an OOBMessage
func (oob *OOBMessage) String() string {
	return fmt.Sprintf("from %s:%d to %s:%d: %d bytes",
		oob.SourceAddr, oob.SourcePort, oob.DestAddr, oob.DestPort, len(oob.Data))
}

// unmarshalOOB unmarshals an OOBMessage from a Msg
func (m Msg) unmarshalOOB() (*OOBMessage, error) {
	if len(m) < oobHeaderData {
		return nil, fmt.Errorf("message too short")
	}
	if m[0] != byte(MsgTypeOOB) {
		return nil, fmt.Errorf("message is not an OOBMessage")
	}
	return &OOBMessage{
		Hops:       m[oobHeaderHops],
		SourceAddr: IP(m[oobHeaderSourceAddr : oobHeaderSourceAddr+16]),
		SourcePort: binary.BigEndian.Uint16(m[oobHeaderSourcePort : oobHeaderSourcePort+2]),
		DestAddr:   IP(m[oobHeaderDestAddr : oobHeaderDestAddr+16]),
		DestPort:   binary.BigEndian.Uint16(m[oobHeaderDestPort : oobHeaderDestPort+2]),
		Data:       m[oobHeaderData:],
	}, nil
}

func (ru *RoutingUpdate) String() string {
	return fmt.Sprintf("%s/%d/%d", ru.Origin.String(), ru.UpdateEpoch, ru.UpdateSequence)
}

//goland:noinspection GoMixedReceiverTypes
func (rc RoutingConns) MarshalJSON() ([]byte, error) {
	m := make(map[string]float32)
	for k, v := range rc {
		m[k.String()] = v
	}
	return json.Marshal(m)
}

//goland:noinspection GoMixedReceiverTypes
func (rc *RoutingConns) UnmarshalJSON(data []byte) error {
	m := make(map[string]float32)
	err := json.Unmarshal(data, &m)
	if err != nil {
		return err
	}
	newRc := make(RoutingConns)
	for k, v := range m {
		var subnet Subnet
		_, subnet, err = ParseCIDR(k)
		if err != nil {
			return err
		}
		newRc[subnet] = v
	}
	*rc = newRc
	return nil
}
