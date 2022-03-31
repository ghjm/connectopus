package netopus

import (
	"encoding/json"
	"fmt"
	"net"
)

// Data structures for the Netopus wire protocol

type Msg []byte

type MsgType int
const (
	MsgTypeError MsgType = -1
	MsgTypeData = 0
	MsgTypeInit = 1
	MsgTypeRoute = 2
	MaxMsgType = 2
)

func (m Msg) Type() MsgType {
	if len(m) < 1 {
		return MsgTypeError
	}
	b := m[0]
	switch {
	case b>>4 == 6:  // this is an ipv6 header
		return MsgTypeData
	case b <= MaxMsgType:
		return MsgType(m[0])
	default:
		return MsgTypeError
	}
}

type InitMsg struct {
	MyAddr net.IP
}

var ErrIncorrectMessageType = fmt.Errorf("incorrect message type")

// UnmarshalInitMsg returns an InitMsg struct from a []byte
func (m Msg) UnmarshalInitMsg() (*InitMsg, error) {
	if m.Type() != MsgTypeInit {
		return nil, ErrIncorrectMessageType
	}
	im := &InitMsg{}
	err := json.Unmarshal(m[1:], im)
	if err != nil {
		return nil, err
	}
	return im, nil
}

// Marshal marshals an InitMsg to a []byte
func (m *InitMsg) Marshal() ([]byte, error) {
	d, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	d = append([]byte{MsgTypeInit}, d...)
	return d, nil
}
