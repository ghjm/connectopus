package netopus

import (
	"context"
	"github.com/ghjm/connectopus/pkg/backends/backend_pair"
	"github.com/ghjm/connectopus/pkg/proto"
	"go.uber.org/goleak"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestOOB(t *testing.T) {
	goleak.VerifyNone(t)
	//ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	n1, err := New(ctx, proto.ParseIP("FD00::1"), "test1", 1400)
	if err != nil {
		t.Fatal(err)
	}
	var n2 proto.Netopus
	n2, err = New(ctx, proto.ParseIP("FD00::2"), "test2", 1400)
	if err != nil {
		t.Fatal(err)
	}
	err = backend_pair.RunPair(ctx, n1, n2, 1500)
	if err != nil {
		t.Fatal(err)
	}
	wg := sync.WaitGroup{}
	wg.Add(2)
	for _, n := range []proto.Netopus{n1, n2} {
		go func(n proto.Netopus) {
			defer wg.Done()
			np := n.(*netopus)
			var remoteIP proto.Subnet
			if np.addr == proto.ParseIP("FD00::1") {
				remoteIP = proto.NewHostOnlySubnet(proto.ParseIP("FD00::2"))
			} else {
				remoteIP = proto.NewHostOnlySubnet(proto.ParseIP("FD00::1"))
			}
			updCh := n.SubscribeUpdates()
			defer n.UnsubscribeUpdates(updCh)
			for {
				select {
				case <-ctx.Done():
					return
				case upd := <-updCh:
					_, ok := upd[remoteIP]
					if ok {
						return
					}
				}
			}
		}(n)
	}
	wg.Wait()
	if ctx.Err() != nil {
		t.Errorf("context cancelled")
		return
	}

	message := []byte("Hello, world!")
	recvChan := make(chan struct{})

	// Test OOBPacketConn
	for _, ns := range [][]proto.Netopus{{n1, n2}, {n2, n1}} {
		nLocal := ns[0]
		nRemote := ns[1]

		// Test OOBPacketConn
		var pcSender net.PacketConn
		pcSender, err = nLocal.NewOOBPacketConn(ctx, 0)
		if err != nil {
			t.Fatal(err)
		}
		var pcReceiver net.PacketConn
		pcReceiver, err = nRemote.NewOOBPacketConn(ctx, 1234)
		if err != nil {
			t.Fatal(err)
		}
		go func() {
			buf := make([]byte, 256)
			n, addr, err := pcReceiver.ReadFrom(buf)
			if err != nil {
				t.Errorf(err.Error())
				return
			}
			if addr != pcSender.LocalAddr() {
				t.Errorf("received message from wrong address")
				return
			}
			buf = buf[:n]
			if string(buf) != string(message) {
				t.Errorf("incorrect message received")
				return
			}
			recvChan <- struct{}{}
		}()
		go func() {
			n, err := pcSender.WriteTo(message, pcReceiver.LocalAddr())
			if err != nil {
				t.Errorf(err.Error())
				return
			}
			if n != len(message) {
				t.Errorf("whole message not sent")
				return
			}
		}()
	}

	completed := 0
	for completed < 2 {
		select {
		case <-ctx.Done():
			t.Errorf(ctx.Err().Error())
			return
		case <-recvChan:
			completed += 1
		}
	}

	// Test OOBConn
	message = []byte(strings.Repeat("Hello, world!\n", 1000))
	for _, ns := range [][]proto.Netopus{{n1, n2}, {n2, n1}} {
		nLocal := ns[0]
		nRemote := ns[1]
		// Test OOBConn
		var li net.Listener
		li, err = nLocal.ListenOOB(ctx, 2345)
		if err != nil {
			t.Fatal(err)
		}
		go func() {
			c, err := li.Accept()
			if err != nil {
				t.Errorf("accept error: %s", err)
				return
			}
			var recvMessage []byte
			for {
				buf := make([]byte, 32768)
				n, err := c.Read(buf)
				if err != nil {
					t.Errorf("error reading message: %s", err)
					return
				}
				buf = buf[:n]
				recvMessage = append(recvMessage, buf...)
				if len(recvMessage) >= len(message) {
					break
				}
			}
			if string(recvMessage) != string(message) {
				t.Errorf("incorrect message received")
				return
			}
			err = c.Close()
			if err != nil {
				t.Errorf("error closing conn: %s", err)
				return
			}
			err = li.Close()
			if err != nil {
				t.Errorf("error closing listener: %s", err)
				return
			}
			recvChan <- struct{}{}
		}()
		go func() {
			c, err := nRemote.DialOOB(ctx, proto.OOBAddr{Host: nLocal.(*netopus).addr, Port: 2345})
			if err != nil {
				t.Errorf(err.Error())
				return
			}
			defer func() {
				time.Sleep(50 * time.Millisecond) // allow ACK to arrive
				err := c.Close()
				if err != nil {
					t.Errorf(err.Error())
					return
				}
			}()
			n, err := c.Write(message)
			if err != nil {
				t.Errorf("error writing to conn: %s", err)
				return
			}
			if n != len(message) {
				t.Errorf("whole message not sent")
				return
			}
		}()

	}

	completed = 0
	for completed < 2 {
		select {
		case <-ctx.Done():
			t.Errorf(ctx.Err().Error())
			return
		case <-recvChan:
			completed += 1
		}
	}

}
