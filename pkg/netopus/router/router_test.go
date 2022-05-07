package router

import (
	"context"
	"go.uber.org/goleak"
	"testing"
	"time"
)

type testNodes map[string]map[string]float32
type testResults map[string]string
type testInstance struct {
	myNode string
	nodes  testNodes
	tests  testResults
}

var testInstances = []testInstance{
	{"me",
		testNodes{
			"me": {"2": 1.0},
			"2":  {"me": 1.0, "3": 1.0},
			"3":  {"2": 1.0},
		},
		testResults{
			"me": "",
			"2":  "2",
			"3":  "2",
		},
	},
	{"me",
		testNodes{
			"me": {"2": 1.0, "3": 1.0},
			"2":  {"me": 1.0, "3": 1.0},
			"3":  {"2": 1.0, "me": 1.0},
		},
		testResults{
			"me": "",
			"2":  "2",
			"3":  "3",
		},
	},
	{"me",
		testNodes{
			"me": {"2": 1.0, "4": 2.0},
			"2":  {"me": 1.0, "3": 1.0, "4": 0.5},
			"3":  {"5": 1.0, "2": 1.0},
			"4":  {"me": 2.0, "2": 0.5, "5": 2.0},
			"5":  {"3": 1.0, "4": 2.0},
		},
		testResults{
			"me": "",
			"2":  "2",
			"3":  "2",
			"4":  "2",
			"5":  "2",
		},
	},
	{"S",
		testNodes{
			"S": {"A": 3.0, "C": 2.0, "F": 6.0},
			"A": {"B": 6.0, "D": 0.5},
			"B": {"E": 1.0},
			"C": {"A": 2.0, "D": 3.0},
			"D": {"E": 4.0},
			"E": {},
			"F": {"E": 2.0},
		},
		testResults{
			"S": "",
			"A": "A",
			"B": "A",
			"C": "C",
			"D": "A",
			"E": "A",
			"F": "F",
		},
	},
}

func doTest(t *testing.T, instance testInstance) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r := New(ctx, instance.myNode, 50*time.Millisecond)
	for node, conns := range instance.nodes {
		r.UpdateNode(node, conns)
	}
	updCh := r.SubscribeUpdates()
	defer r.UnsubscribeUpdates(updCh)
	select {
	case <-ctx.Done():
	case policy := <-updCh:
		for dest, result := range instance.tests {
			hop := policy[dest]
			if hop != result {
				t.Errorf("router produced incorrect result")
			}
		}
	}
}

func TestRouter(t *testing.T) {
	defer goleak.VerifyNone(t)
	for _, instance := range testInstances {
		doTest(t, instance)
	}
}
