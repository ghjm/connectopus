package netopus

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/pkg/backends/backend_pair"
	log "github.com/sirupsen/logrus"
	"go.uber.org/goleak"
	"io"
	"net"
	"os"
	"sync"
	"testing"
	"time"
)

// NodeSpec defines a node (Netopus instance) in a mesh
type NodeSpec struct {
	Address net.IP
	Conns   []string
}

// ConnSpec defines a connection between two named nodes
type ConnSpec struct {
	N1 string
	N2 string
}

// MakeMesh constructs Netopus instances and backends, according to a spec
func MakeMesh(ctx context.Context, meshSpec map[string]NodeSpec) (map[string]Netopus, error) {
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
	mesh := make(map[string]Netopus)
	usedAddrs := make(map[string]struct{})
	for node, spec := range meshSpec {
		addrStr := spec.Address.String()
		_, ok := usedAddrs[addrStr]
		if ok {
			return nil, fmt.Errorf("duplicate address in spec")
		}
		usedAddrs[addrStr] = struct{}{}
		n, err := NewNetopus(ctx, spec.Address)
		if err != nil {
			return nil, err
		}
		mesh[node] = n
	}
	for _, conn := range connections {
		err := backend_pair.RunPair(ctx, mesh[conn.N1], mesh[conn.N2], 1500)
		if err != nil {
			return nil, err
		}
	}
	startTime := time.Now()
	for _, conn := range connections {
		allGood := true
		for n, c := range map[string]string{conn.N1: conn.N2, conn.N2: conn.N1} {
			good := false
			mesh[n].(*netopus).sessionInfo.WorkWithReadOnly(func(s *sessInfo) {
				for _, v := range s.sessions {
					if v.connected.Get() && v.remoteAddr.Get().Equal(meshSpec[c].Address) {
						good = true
						return
					}
				}
			})
			if !good {
				allGood = false
				break
			}
		}
		if allGood {
			break
		}
		if time.Now().Sub(startTime) > 5*time.Second {
			return nil, fmt.Errorf("timeout initializing mesh")
		}
		time.Sleep(100 * time.Millisecond)
	}
	return mesh, nil
}

func stackTest(ctx context.Context, t *testing.T, spec map[string]NodeSpec, mesh map[string]Netopus) {
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
	nConns := 5 //TODO: This fails with larger connection counts.  See https://github.com/google/gvisor/issues/7379
	wg := sync.WaitGroup{}
	wg.Add(2 * nConns)

	// Connect using TCP
	for i := 0; i < nConns; i++ {
		go func(id int) {
			defer func() {
				wg.Done()
			}()
			c, err := client.DialContextTCP(ctx, serverAddr, 1234)
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
			udc, err := client.DialUDP(0, serverAddr, 2345)
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
					select {
					case <-pingCtx.Done():
						return
					case <-time.After(time.Second):
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

func idleTest(ctx context.Context, t *testing.T, spec map[string]NodeSpec, mesh map[string]Netopus) {
}

func runTest(t *testing.T, spec map[string]NodeSpec,
	tests ...func(context.Context, *testing.T, map[string]NodeSpec, map[string]Netopus)) {
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
		"server": {net.ParseIP("FD00::1"), []string{"client"}},
		"client": {net.ParseIP("FD00::2"), []string{"server"}},
	}, stackTest)
	t.Logf("3 node test, linear\n")
	runTest(t, map[string]NodeSpec{
		"server": {net.ParseIP("FD00::1"), []string{"A"}},
		"client": {net.ParseIP("FD00::2"), []string{"A"}},
		"A":      {net.ParseIP("FD00::3"), []string{"server", "client"}},
	}, stackTest)
	t.Logf("3 node test, circular\n")
	runTest(t, map[string]NodeSpec{
		"server": {net.ParseIP("FD00::1"), []string{"A", "client"}},
		"client": {net.ParseIP("FD00::2"), []string{"server", "A"}},
		"A":      {net.ParseIP("FD00::3"), []string{"server", "client"}},
	}, stackTest)
	t.Logf("4 node test\n")
	runTest(t, map[string]NodeSpec{
		"server": {net.ParseIP("FD00::1"), []string{"A", "B"}},
		"client": {net.ParseIP("FD00::2"), []string{"A", "B"}},
		"A":      {net.ParseIP("FD00::3"), []string{"server", "client"}},
		"B":      {net.ParseIP("FD00::4"), []string{"server", "client"}},
	}, stackTest)
	t.Logf("8 node test, linear\n")
	runTest(t, map[string]NodeSpec{
		"server": {net.ParseIP("FD00::1"), []string{"A"}},
		"A":      {net.ParseIP("FD00::2"), []string{"server", "B"}},
		"B":      {net.ParseIP("FD00::3"), []string{"A", "C"}},
		"C":      {net.ParseIP("FD00::4"), []string{"B", "D"}},
		"D":      {net.ParseIP("FD00::5"), []string{"C", "E"}},
		"E":      {net.ParseIP("FD00::6"), []string{"D", "F"}},
		"F":      {net.ParseIP("FD00::7"), []string{"E", "client"}},
		"client": {net.ParseIP("FD00::8"), []string{"F"}},
	}, stackTest)
	t.Logf("8 node test, circular\n")
	runTest(t, map[string]NodeSpec{
		"server": {net.ParseIP("FD00::1"), []string{"A"}},
		"client": {net.ParseIP("FD00::2"), []string{"F"}},
		"A":      {net.ParseIP("FD00::3"), []string{"B", "C", "server"}},
		"B":      {net.ParseIP("FD00::4"), []string{"A", "D"}},
		"C":      {net.ParseIP("FD00::5"), []string{"A", "E"}},
		"D":      {net.ParseIP("FD00::6"), []string{"B", "F"}},
		"E":      {net.ParseIP("FD00::7"), []string{"C", "F"}},
		"F":      {net.ParseIP("FD00::8"), []string{"D", "E", "client"}},
	}, stackTest, idleTest)
}
