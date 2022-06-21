package netopus

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/pkg/backends/backend_pair"
	"github.com/ghjm/connectopus/pkg/proto"
	log "github.com/sirupsen/logrus"
	"go.uber.org/goleak"
	"io"
	"net"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"
)

// NodeSpec defines a node (Netopus instance) in a mesh
type NodeSpec struct {
	Address proto.IP
	Conns   []string
}

// ConnSpec defines a connection between two named nodes
type ConnSpec struct {
	N1 string
	N2 string
}

// MakeMesh constructs Netopus instances and backends, according to a spec
func MakeMesh(ctx context.Context, meshSpec map[string]NodeSpec) (map[string]*netopus, error) {
	connections := make([]ConnSpec, 0)
	for node, spec := range meshSpec {
		for _, conn := range spec.Conns {
			remNode, ok := meshSpec[conn]
			if !ok {
				return nil, fmt.Errorf("node %s connects to non-existing node %s", node, conn)
			}
			found := false
			for _, remConn := range remNode.Conns {
				if remConn == node {
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("node %s connects to %s with no reverse connection", node, conn)
			}
			found = false
			for _, b := range connections {
				if (b.N1 == node && b.N2 == conn) || (b.N1 == conn && b.N2 == node) {
					found = true
					break
				}
			}
			if !found {
				connections = append(connections, ConnSpec{N1: node, N2: conn})
			}
		}
	}
	mesh := make(map[string]*netopus)
	usedAddrs := make(map[string]struct{})
	for node, spec := range meshSpec {
		addrStr := spec.Address.String()
		_, ok := usedAddrs[addrStr]
		if ok {
			return nil, fmt.Errorf("duplicate address in spec")
		}
		usedAddrs[addrStr] = struct{}{}
		n, err := New(ctx, spec.Address, "test")
		if err != nil {
			return nil, err
		}
		mesh[node] = n.(*netopus)
	}
	for _, conn := range connections {
		err := backend_pair.RunPair(ctx, mesh[conn.N1], mesh[conn.N2], 1500)
		if err != nil {
			return nil, err
		}
	}
	timeoutCtx, timeoutCancel := context.WithTimeout(ctx, 15*time.Second)
	defer timeoutCancel()
	wg := sync.WaitGroup{}
	wg.Add(len(meshSpec))
	for node := range meshSpec {
		go func(node string) {
			defer wg.Done()
			updCh := mesh[node].SubscribeUpdates()
			defer mesh[node].UnsubscribeUpdates(updCh)
			for {
				select {
				case <-timeoutCtx.Done():
					return
				case policy := <-updCh:
					allGood := true
					for nodeB := range meshSpec {
						if nodeB == node {
							continue
						}
						found := false
						for s := range policy {
							if s.Contains(mesh[nodeB].addr) {
								found = true
								break
							}
						}
						if !found {
							allGood = false
							break
						}
					}
					if allGood {
						return
					}
				}
			}
		}(node)
	}
	wg.Wait()
	if timeoutCtx.Err() != nil {
		return nil, timeoutCtx.Err()
	}
	return mesh, nil
}

func stackTest(ctx context.Context, t *testing.T, spec map[string]NodeSpec, mesh map[string]*netopus) {
	server := mesh["server"]
	serverAddr := spec["server"].Address
	client := mesh["client"]
	testStr := "Hello, world!"

	// Start TCP listener
	li, err := server.ListenTCP(1234)
	if err != nil {
		t.Fatalf("listen TCP error: %s", err)
	}
	go func() {
		defer func() {
			_ = li.Close()
		}()
		for {
			sc, err := li.Accept()
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				t.Errorf("accept error: %s", err)
				return
			}
			go func() {
				_, _ = sc.Write([]byte(testStr))
				//_ = sc.(*gonet.TCPConn).CloseWrite()
				_ = sc.Close()
			}()
		}
	}()

	// Start UDP listener
	ulc, err := server.DialUDP(2345, nil, 0)
	if err != nil {
		t.Fatalf("UDP listener error: %s", err)
	}
	go func() {
		defer func() {
			_ = ulc.Close()
		}()
		for {
			buf := make([]byte, 1500)
			if err != nil {
				t.Errorf("Error setting read deadline: %s", err)
				return
			}
			_, addr, err := ulc.ReadFrom(buf)
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				t.Errorf("UDP read error: %s", err)
				return
			}
			go func() {
				n, err := ulc.WriteTo([]byte(testStr), addr)
				if err != nil {
					t.Errorf("UDP write error: %s", err)
					return
				}
				if n != len(testStr) {
					t.Errorf("UDP expected to write %d bytes but wrote %d", len(testStr), n)
				}
			}()
		}
	}()

	// Set up wait group
	nConns := 10
	if runtime.GOOS == "linux" {
		// Code is less optimized for other platforms
		nConns = 100
	}
	wg := sync.WaitGroup{}
	wg.Add(2 * nConns)

	// Connect using TCP
	for i := 0; i < nConns; i++ {
		go func(id int) {
			defer func() {
				wg.Done()
			}()
			c, err := client.DialContextTCP(ctx, net.IP(serverAddr), 1234)
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				t.Errorf("dial TCP error: %s", err)
				return
			}
			b, err := io.ReadAll(c)
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				t.Errorf("read TCP error: %s", err)
				return
			}
			err = c.Close()
			if err != nil && ctx.Err() == nil {
				t.Errorf("close TCP error: %s", err)
				return
			}
			if string(b) != testStr {
				t.Errorf("incorrect data received: expected %s but got %s", testStr, b)
				return
			}
		}(i)
	}

	// Connect using UDP
	for i := 0; i < nConns; i++ {
		go func(id int) {
			defer func() {
				wg.Done()
			}()
			udc, err := client.DialUDP(0, net.IP(serverAddr), 2345)
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				t.Errorf("dial UDP error: %s", err)
				return
			}
			pingCtx, pingCancel := context.WithCancel(ctx)
			go func() {
				for {
					_, err := udc.Write([]byte("ping"))
					if err != nil && pingCtx.Err() == nil {
						t.Errorf("UDP write error: %s", err)
						return
					}
					timer := time.NewTimer(time.Second)
					select {
					case <-pingCtx.Done():
						timer.Stop()
						return
					case <-timer.C:
					}
				}
			}()
			b := make([]byte, 1500)
			n, err := udc.Read(b)
			pingCancel()
			if err != nil {
				t.Errorf("UDP read error: %s", err)
				return
			}
			if n != len(testStr) || string(b[:n]) != testStr {
				t.Errorf("UDP read data incorrect: expected %s, got %s", testStr, b[:n])
				return
			}
			err = udc.Close()
			if err != nil {
				t.Errorf("close UDP error: %s", err)
				return
			}
		}(i)
	}

	// Wait for completion
	wg.Wait()
}

func idleTest(_ context.Context, _ *testing.T, _ map[string]NodeSpec, _ map[string]*netopus) {
}

func runTest(t *testing.T, spec map[string]NodeSpec,
	tests ...func(context.Context, *testing.T, map[string]NodeSpec, map[string]*netopus)) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer func() {
		cancel()
	}()
	mesh, err := MakeMesh(ctx, spec)
	if err != nil {
		t.Fatalf("mesh initialization error: %s", err)
	}
	for _, test := range tests {
		test(ctx, t, spec, mesh)
	}
}

func TestNetopus(t *testing.T) {
	defer goleak.VerifyNone(t)
	//log.SetLevel(log.DebugLevel)
	log.SetOutput(os.Stdout)
	t.Logf("2 node test\n")
	runTest(t, map[string]NodeSpec{
		"server": {proto.ParseIP("FD00::1"), []string{"client"}},
		"client": {proto.ParseIP("FD00::2"), []string{"server"}},
	}, stackTest)
	t.Logf("3 node test, linear\n")
	runTest(t, map[string]NodeSpec{
		"server": {proto.ParseIP("FD00::1"), []string{"A"}},
		"client": {proto.ParseIP("FD00::2"), []string{"A"}},
		"A":      {proto.ParseIP("FD00::3"), []string{"server", "client"}},
	}, stackTest)
	t.Logf("3 node test, circular\n")
	runTest(t, map[string]NodeSpec{
		"server": {proto.ParseIP("FD00::1"), []string{"A", "client"}},
		"client": {proto.ParseIP("FD00::2"), []string{"server", "A"}},
		"A":      {proto.ParseIP("FD00::3"), []string{"server", "client"}},
	}, stackTest)
	t.Logf("4 node test\n")
	runTest(t, map[string]NodeSpec{
		"server": {proto.ParseIP("FD00::1"), []string{"A", "B"}},
		"client": {proto.ParseIP("FD00::2"), []string{"A", "B"}},
		"A":      {proto.ParseIP("FD00::3"), []string{"server", "client"}},
		"B":      {proto.ParseIP("FD00::4"), []string{"server", "client"}},
	}, stackTest)
	t.Logf("8 node test, linear\n")
	runTest(t, map[string]NodeSpec{
		"server": {proto.ParseIP("FD00::1"), []string{"A"}},
		"A":      {proto.ParseIP("FD00::2"), []string{"server", "B"}},
		"B":      {proto.ParseIP("FD00::3"), []string{"A", "C"}},
		"C":      {proto.ParseIP("FD00::4"), []string{"B", "D"}},
		"D":      {proto.ParseIP("FD00::5"), []string{"C", "E"}},
		"E":      {proto.ParseIP("FD00::6"), []string{"D", "F"}},
		"F":      {proto.ParseIP("FD00::7"), []string{"E", "client"}},
		"client": {proto.ParseIP("FD00::8"), []string{"F"}},
	}, stackTest)
	t.Logf("8 node test, circular\n")
	runTest(t, map[string]NodeSpec{
		"server": {proto.ParseIP("FD00::1"), []string{"A"}},
		"client": {proto.ParseIP("FD00::2"), []string{"F"}},
		"A":      {proto.ParseIP("FD00::3"), []string{"B", "C", "server"}},
		"B":      {proto.ParseIP("FD00::4"), []string{"A", "D"}},
		"C":      {proto.ParseIP("FD00::5"), []string{"A", "E"}},
		"D":      {proto.ParseIP("FD00::6"), []string{"B", "F"}},
		"E":      {proto.ParseIP("FD00::7"), []string{"C", "F"}},
		"F":      {proto.ParseIP("FD00::8"), []string{"D", "E", "client"}},
	}, stackTest, idleTest)
}
