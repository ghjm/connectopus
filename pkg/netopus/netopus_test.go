package netopus

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/pkg/backends/backend_pair"
	"github.com/ghjm/connectopus/pkg/utils"
	log "github.com/sirupsen/logrus"
	"go.uber.org/goleak"
	"io"
	"net"
	"os"
	"sync"
	"testing"
	"time"
)

var testStr = "Hello, world!"

func TestNetopus(t *testing.T) {
	defer goleak.VerifyNone(t)
	log.SetLevel(log.DebugLevel)
	log.SetOutput(os.Stdout)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	n1addr := net.ParseIP("FD00::1")
	n1, err := NewNetopus(ctx, n1addr)
	if err != nil {
		t.Fatalf("netopus initialization error %s", err)
	}
	n2addr := net.ParseIP("FD00::2")
	n2, err := NewNetopus(ctx, n2addr)
	if err != nil {
		t.Fatalf("netopus initialization error %s", err)
	}

	err = backend_pair.RunPair(ctx, n1, n2, 1500)
	if err != nil {
		t.Fatalf("pair backend error %s", err)
	}

	// Wait for connection
	checkConn := func(n Netopus) bool {
		tr := n.(*netopus).sessions.BeginTransaction()  //TODO: fix this after implementing netopus.Status()
		defer tr.EndTransaction()
		m := *tr.RawMap()
		if len(m) == 0 {
			return false
		}
		for _, v := range m {
			if v.connected.Get() {
				return true
			}
		}
		return false
	}
	connGood := false
	for i := 0; i < 10; i++ {
		if checkConn(n1) && checkConn(n2) {
			connGood = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !connGood {
		t.Fatalf("Netopus instances didn't connect")
	}

	// Start TCP listener
	li, err := n1.ListenTCP(1234)
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
	ulc, err := n1.DialUDP(2345, nil, 0)
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
	nConns := 10  //TODO: This fails with larger connection counts.  See https://github.com/google/gvisor/issues/7379
	wg := sync.WaitGroup{}
	wg.Add(2*nConns)

	// Connect using TCP
	for i := 0; i < nConns; i++ {
		go func(id int) {
			defer func() {
				wg.Done()
			}()
			c, err := n2.DialContextTCP(ctx, n1addr, 1234)
			if err != nil {
				t.Errorf("dial TCP error: %s", err)
				return
			}
			if ctx.Err() != nil {
				return
			}
			b, err := io.ReadAll(c)
			if err != nil {
				t.Errorf("read TCP error: %s", err)
				return
			}
			err = c.Close()
			if err != nil {
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
			udc, err := n2.DialUDP(0, n1addr, 2345)
			if err != nil {
				t.Errorf("dial UDP error: %s", err)
				return
			}
			if ctx.Err() != nil {
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
			timeCancel := utils.LogTime(ctx, fmt.Sprintf("UDP read #%d", id), time.Second)
			n, err := udc.Read(b)
			timeCancel()
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
