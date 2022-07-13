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

func TestDirect(t *testing.T) {
	goleak.VerifyNone(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var pc1, pc2 *OOBPacketConn
	ip1 := func(msg *proto.OOBMessage) error {
		if pc1 != nil {
			return pc1.IncomingPacket(msg)
		}
		return nil
	}
	ip2 := func(msg *proto.OOBMessage) error {
		if pc2 != nil {
			return pc2.IncomingPacket(msg)
		}
		return nil
	}
	var err error
	pc1, err = NewPacketConn(ctx, ip2, nil, proto.ParseIP("FD00::1"), 1234)
	if err != nil {
		t.Fatal(err)
	}
	pc2, err = NewPacketConn(ctx, ip1, nil, proto.ParseIP("FD00::2"), 1234)
	if err != nil {
		t.Fatal(err)
	}
	message := []byte("Hello, world!")
	wg := sync.WaitGroup{}
	wg.Add(4)
	writePacket := func(pc net.PacketConn, addr proto.IP) {
		defer wg.Done()
		n, err := pc.WriteTo(message, proto.OOBAddr{Host: addr, Port: 1234})
		if err != nil {
			t.Errorf("write error: %s", err)
			return
		}
		if n != len(message) {
			t.Errorf("whole message not sent")
			return
		}
	}
	go writePacket(pc1, proto.ParseIP("FD00::2"))
	go writePacket(pc2, proto.ParseIP("FD00::1"))
	readPacket := func(pc net.PacketConn, addr proto.IP) {
		defer wg.Done()
		buf := make([]byte, 256)
		n, raddr, err := pc.ReadFrom(buf)
		if err != nil {
			t.Errorf("read error: %s", err)
			return
		}
		if string(buf[:n]) != string(message) {
			t.Errorf("incorrect message received")
			return
		}
		if raddr.String() != net.JoinHostPort(addr.String(), "1234") {
			t.Errorf("message from wrong address")
			return
		}
	}
	go readPacket(pc1, proto.ParseIP("FD00::2"))
	go readPacket(pc2, proto.ParseIP("FD00::1"))
	wg.Wait()
}

func TestOOB(t *testing.T) {
	goleak.VerifyNone(t)
	//ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	n1, err := New(ctx, proto.ParseIP("FD00::1"), "test1", WithMTU(1400))
	if err != nil {
		t.Fatal(err)
	}
	var n2 proto.Netopus
	n2, err = New(ctx, proto.ParseIP("FD00::2"), "test2", WithMTU(1400))
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
	longMessage := []byte(strings.Repeat("Hello, world!\n", 1000))
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
				if len(recvMessage) >= len(longMessage) {
					break
				}
			}
			if string(recvMessage) != string(longMessage) {
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
			n, err := c.Write(longMessage)
			if err != nil {
				t.Errorf("error writing to conn: %s", err)
				return
			}
			if n != len(longMessage) {
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
