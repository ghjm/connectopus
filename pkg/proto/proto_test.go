package proto

import (
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"net"
	"reflect"
	"testing"
)

func TestType(t *testing.T) {
	b := make([]byte, header.IPv6MinimumSize)
	header.IPv6(b).Encode(&header.IPv6Fields{
		TrafficClass:      0xff,
		FlowLabel:         0xff,
		PayloadLength:     0,
		TransportProtocol: 0,
		HopLimit:          0,
		SrcAddr:           tcpip.Address(net.IPv6loopback),
		DstAddr:           tcpip.Address(net.IPv6loopback),
		ExtensionHeaders:  nil,
	})
	if Msg(b).Type() != MsgTypeData {
		t.Errorf("ipv6 header not detected as data")
	}
	for mt := range []MsgType{MsgTypeInit, MsgTypeRoute} {
		b[0] = byte(mt)
		if Msg(b).Type() != MsgType(mt) {
			t.Errorf("message type %d detected wrong", mt)
		}
	}
	b[0] = byte(MaxMsgType + 1)
	if Msg(b).Type() != MsgTypeError {
		t.Errorf("invalid message type did not produce error")
	}
}

func TestInitMsg(t *testing.T) {
	im := &InitMsg{MyAddr: ParseIP("FE00::1")}
	b, err := im.Marshal()
	if err != nil {
		t.Errorf("error marshaling InitMsg: %s", err)
	}
	im2, err := Msg(b).Unmarshal()
	if err != nil {
		t.Errorf("error unmarshaling InitMsg: %s", err)
	}
	if !reflect.DeepEqual(im, im2) {
		t.Errorf("round trip error marshaling/unmarshaling InitMsg")
	}
}

func TestRoutingUpdate(t *testing.T) {
	r := &RoutingUpdate{
		Origin:         ParseIP("FD00::1"),
		UpdateEpoch:    5678,
		UpdateSequence: 9012,
		Connections: map[Subnet]float32{
			NewHostOnlySubnet(ParseIP("FD00::1")): 1.0,
		},
	}
	b, err := r.Marshal()
	if err != nil {
		t.Errorf("error marshaling RoutingUpdate: %s", err)
	}
	r2, err := Msg(b).Unmarshal()
	if err != nil {
		t.Errorf("error unmarshaling RoutingUpdate: %s", err)
	}
	if !reflect.DeepEqual(r, r2) {
		t.Errorf("round trip error marshaling/unmarshaling RoutingUpdate")
	}
}
