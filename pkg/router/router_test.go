package router

import (
	"context"
	"github.com/ghjm/connectopus/pkg/proto"
	"go.uber.org/goleak"
	"testing"
	"time"
)

type testNodes proto.RoutingNodes
type testResults map[proto.IP]proto.IP
type testInstance struct {
	myAddr proto.IP
	nodes  testNodes
	tests  testResults
}

var tcpipAddresses = map[string]proto.IP{
	"me":   proto.ParseIP("fd00::1"),
	"2":    proto.ParseIP("fd00::2"),
	"3":    proto.ParseIP("fd00::3"),
	"4":    proto.ParseIP("fd00::4"),
	"5":    proto.ParseIP("fd00::5"),
	"S":    proto.ParseIP("fd00::1:0"),
	"A":    proto.ParseIP("fd00::1:1"),
	"B":    proto.ParseIP("fd00::1:2"),
	"C":    proto.ParseIP("fd00::1:3"),
	"D":    proto.ParseIP("fd00::1:4"),
	"E":    proto.ParseIP("fd00::1:5"),
	"F":    proto.ParseIP("fd00::1:6"),
	"X1":   proto.ParseIP("fd00:1::1"),
	"X2":   proto.ParseIP("fd00:2::1"),
	"v4_0": proto.ParseIP("192.168.0.1"),
	"v4_1": proto.ParseIP("192.168.1.1"),
}

var tcpipSubnets = map[string]proto.Subnet{
	"me":        proto.NewHostOnlySubnet(tcpipAddresses["me"]),
	"2":         proto.NewHostOnlySubnet(tcpipAddresses["2"]),
	"3":         proto.NewHostOnlySubnet(tcpipAddresses["3"]),
	"4":         proto.NewHostOnlySubnet(tcpipAddresses["4"]),
	"5":         proto.NewHostOnlySubnet(tcpipAddresses["5"]),
	"S":         proto.NewHostOnlySubnet(tcpipAddresses["S"]),
	"A":         proto.NewHostOnlySubnet(tcpipAddresses["A"]),
	"B":         proto.NewHostOnlySubnet(tcpipAddresses["B"]),
	"C":         proto.NewHostOnlySubnet(tcpipAddresses["C"]),
	"D":         proto.NewHostOnlySubnet(tcpipAddresses["D"]),
	"E":         proto.NewHostOnlySubnet(tcpipAddresses["E"]),
	"F":         proto.NewHostOnlySubnet(tcpipAddresses["F"]),
	"X1":        proto.NewSubnet(tcpipAddresses["X1"], proto.CIDRMask(64, 128)),
	"X2":        proto.NewSubnet(tcpipAddresses["X2"], proto.CIDRMask(64, 128)),
	"X1_long":   proto.NewSubnet(tcpipAddresses["X1"], proto.CIDRMask(96, 128)),
	"v4_0":      proto.NewSubnet(tcpipAddresses["v4_0"], proto.CIDRMask(16, 32)),
	"v4_1":      proto.NewSubnet(tcpipAddresses["v4_1"], proto.CIDRMask(16, 32)),
	"v4_1_long": proto.NewSubnet(tcpipAddresses["v4_1"], proto.CIDRMask(24, 32)),
}

var testInstances = []testInstance{
	{tcpipAddresses["me"],
		testNodes{
			tcpipAddresses["me"]: {tcpipSubnets["2"]: 1.0},
			tcpipAddresses["2"]:  {tcpipSubnets["me"]: 1.0, tcpipSubnets["3"]: 1.0},
			tcpipAddresses["3"]:  {tcpipSubnets["2"]: 1.0},
		},
		testResults{
			tcpipAddresses["2"]: tcpipAddresses["2"],
			tcpipAddresses["3"]: tcpipAddresses["2"],
		},
	},
	{tcpipAddresses["me"],
		testNodes{
			tcpipAddresses["me"]: {tcpipSubnets["2"]: 1.0, tcpipSubnets["3"]: 1.0},
			tcpipAddresses["2"]:  {tcpipSubnets["me"]: 1.0, tcpipSubnets["3"]: 1.0},
			tcpipAddresses["3"]:  {tcpipSubnets["2"]: 1.0, tcpipSubnets["me"]: 1.0},
		},
		testResults{
			tcpipAddresses["2"]: tcpipAddresses["2"],
			tcpipAddresses["3"]: tcpipAddresses["3"],
		},
	},
	{tcpipAddresses["me"],
		testNodes{
			tcpipAddresses["me"]: {tcpipSubnets["2"]: 1.0, tcpipSubnets["4"]: 2.0},
			tcpipAddresses["2"]:  {tcpipSubnets["me"]: 1.0, tcpipSubnets["3"]: 1.0, tcpipSubnets["4"]: 0.5},
			tcpipAddresses["3"]:  {tcpipSubnets["5"]: 1.0, tcpipSubnets["2"]: 1.0},
			tcpipAddresses["4"]:  {tcpipSubnets["me"]: 2.0, tcpipSubnets["2"]: 0.5, tcpipSubnets["5"]: 2.0},
			tcpipAddresses["5"]:  {tcpipSubnets["3"]: 1.0, tcpipSubnets["4"]: 2.0},
		},
		testResults{
			tcpipAddresses["2"]: tcpipAddresses["2"],
			tcpipAddresses["3"]: tcpipAddresses["2"],
			tcpipAddresses["4"]: tcpipAddresses["2"],
			tcpipAddresses["5"]: tcpipAddresses["2"],
		},
	},
	{tcpipAddresses["S"],
		testNodes{
			tcpipAddresses["S"]: {tcpipSubnets["A"]: 3.0, tcpipSubnets["C"]: 2.0, tcpipSubnets["F"]: 6.0},
			tcpipAddresses["A"]: {tcpipSubnets["B"]: 6.0, tcpipSubnets["D"]: 0.5},
			tcpipAddresses["B"]: {tcpipSubnets["E"]: 1.0},
			tcpipAddresses["C"]: {tcpipSubnets["A"]: 2.0, tcpipSubnets["D"]: 3.0},
			tcpipAddresses["D"]: {tcpipSubnets["E"]: 4.0},
			tcpipAddresses["E"]: {},
			tcpipAddresses["F"]: {tcpipSubnets["E"]: 2.0},
		},
		testResults{
			tcpipAddresses["A"]: tcpipAddresses["A"],
			tcpipAddresses["B"]: tcpipAddresses["A"],
			tcpipAddresses["C"]: tcpipAddresses["C"],
			tcpipAddresses["D"]: tcpipAddresses["A"],
			tcpipAddresses["E"]: tcpipAddresses["A"],
			tcpipAddresses["F"]: tcpipAddresses["F"],
		},
	},
	{tcpipAddresses["me"],
		testNodes{
			tcpipAddresses["me"]: {tcpipSubnets["2"]: 1.0, tcpipSubnets["4"]: 1.0},
			tcpipAddresses["2"]:  {tcpipSubnets["me"]: 1.0, tcpipSubnets["3"]: 1.0, tcpipSubnets["4"]: 0.5, tcpipSubnets["X1"]: 1.0},
			tcpipAddresses["3"]:  {tcpipSubnets["5"]: 1.0, tcpipSubnets["2"]: 1.0, tcpipSubnets["X1"]: 1.0},
			tcpipAddresses["4"]:  {tcpipSubnets["me"]: 1.0, tcpipSubnets["2"]: 0.5, tcpipSubnets["5"]: 0.5, tcpipSubnets["X2"]: 0.5},
			tcpipAddresses["5"]:  {tcpipSubnets["3"]: 1.0, tcpipSubnets["4"]: 0.5, tcpipSubnets["X2"]: 1.0},
		},
		testResults{
			tcpipAddresses["2"]:  tcpipAddresses["2"],
			tcpipAddresses["3"]:  tcpipAddresses["2"],
			tcpipAddresses["4"]:  tcpipAddresses["4"],
			tcpipAddresses["5"]:  tcpipAddresses["4"],
			tcpipAddresses["X1"]: tcpipAddresses["2"],
			tcpipAddresses["X2"]: tcpipAddresses["4"],
		},
	},
	{tcpipAddresses["me"],
		testNodes{
			tcpipAddresses["me"]: {tcpipSubnets["2"]: 1.0, tcpipSubnets["3"]: 1.0},
			tcpipAddresses["2"]:  {tcpipSubnets["me"]: 1.0, tcpipSubnets["X1_long"]: 2.0},
			tcpipAddresses["3"]:  {tcpipSubnets["me"]: 1.0, tcpipSubnets["X2"]: 1.0},
		},
		testResults{
			tcpipAddresses["2"]:  tcpipAddresses["2"],
			tcpipAddresses["3"]:  tcpipAddresses["3"],
			tcpipAddresses["X1"]: tcpipAddresses["2"],
			tcpipAddresses["X2"]: tcpipAddresses["3"],
		},
	},
	{tcpipAddresses["me"],
		testNodes{
			tcpipAddresses["me"]: {tcpipSubnets["2"]: 1.0, tcpipSubnets["3"]: 1.0},
			tcpipAddresses["2"]:  {tcpipSubnets["me"]: 1.0, tcpipSubnets["v4_0"]: 1.0},
			tcpipAddresses["3"]:  {tcpipSubnets["me"]: 1.0, tcpipSubnets["v4_1_long"]: 2.0},
		},
		testResults{
			tcpipAddresses["2"]:    tcpipAddresses["2"],
			tcpipAddresses["3"]:    tcpipAddresses["3"],
			tcpipAddresses["v4_0"]: tcpipAddresses["2"],
			tcpipAddresses["v4_1"]: tcpipAddresses["3"],
		},
	},
}

func doTest(t *testing.T, instance testInstance) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r := New(ctx, instance.myAddr, 50*time.Millisecond)
	for node, conns := range instance.nodes {
		r.UpdateNode(node, conns)
	}
	updCh := r.SubscribeUpdates()
	defer r.UnsubscribeUpdates(updCh)
	select {
	case <-ctx.Done():
	case <-updCh:
	}
	for dest, result := range instance.tests {
		hop, err := r.NextHop(dest)
		if err != nil {
			t.Errorf("next hop error: %s", err)
		}
		if hop != result {
			t.Errorf("dest %s should be via %s but was via %s", dest.String(), result.String(), hop.String())
		}
	}
}

func TestRouter(t *testing.T) {
	defer goleak.VerifyNone(t)
	for _, instance := range testInstances {
		doTest(t, instance)
	}
}
