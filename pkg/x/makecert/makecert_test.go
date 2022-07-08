package makecert

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"go.uber.org/goleak"
	"net"
	"strings"
	"sync"
	"testing"
)

func TestMakeCert(t *testing.T) {
	goleak.VerifyNone(t)
	ca, err := MakeCA("testing", 1024, 1)
	if err != nil {
		t.Fatal(err)
	}
	var cert *Cert
	cert, err = ca.MakeCert("testing", 1024, 1, nil, []string{"testing"})
	if err != nil {
		t.Fatal(err)
	}
	var li net.Listener
	li, err = tls.Listen("tcp", "", &tls.Config{
		Certificates: []tls.Certificate{cert.TLSCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    ca.Pool,
		MinVersion:   tls.VersionTLS12,
	})
	if err != nil {
		t.Fatal(err)
	}
	message := "Hello, world!"
	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		defer wg.Done()
		c, err := li.Accept()
		if err != nil {
			t.Errorf("accept error: %s", err)
			return
		}
		_, _ = fmt.Fprintf(c, "%s\n", message)
		_ = c.Close()
	}()
	go func() {
		defer wg.Done()
		c, err := tls.Dial("tcp", li.Addr().String(), &tls.Config{
			Certificates: []tls.Certificate{cert.TLSCert},
			RootCAs:      ca.Pool,
			ServerName:   "testing",
			MinVersion:   tls.VersionTLS12,
		})
		if err != nil {
			t.Errorf("dial error: %s", err)
			return
		}
		r := bufio.NewReader(c)
		var s string
		s, err = r.ReadString('\n')
		if err != nil {
			t.Errorf("dial error: %s", err)
			return
		}
		s = strings.TrimSuffix(s, "\n")
		if s != message {
			t.Errorf("incorrect message received")
		}
		_ = c.Close()
	}()
	wg.Wait()
}
