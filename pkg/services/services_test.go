package services

import (
	"bufio"
	"context"
	"fmt"
	"github.com/ghjm/connectopus/pkg/config"
	"github.com/ghjm/connectopus/pkg/netstack"
	"go.uber.org/goleak"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestService(t *testing.T) {
	defer goleak.VerifyNone(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	port := 29876
	svc := config.Service{
		Port:       port,
		Command:    "/bin/cat",
		WinCommand: "find /v \"\"",
	}
	err := RunService(ctx, &netstack.NetUserStack{}, svc)
	if err != nil {
		t.Fatal(err)
	}
	numTests := 10
	wg := sync.WaitGroup{}
	wg.Add(numTests)
	for i := 0; i < 10; i++ {
		go func(n int) {
			defer wg.Done()
			message := fmt.Sprintf("message %d\n", n)
			conn, err := net.Dial("tcp", fmt.Sprintf(":%d", port))
			if err != nil {
				t.Error(err)
				return
			}
			defer func() {
				err = conn.Close()
				if err != nil {
					t.Error(err)
				}
			}()
			_, err = conn.Write([]byte(message))
			if err != nil {
				t.Error(err)
				return
			}
			sr := bufio.NewReader(conn)
			var s string
			s, err = sr.ReadString('\n')
			if err != nil {
				t.Error(err)
				return
			}
			if strings.TrimSpace(s) != strings.TrimSpace(message) {
				t.Error("received message did not match sent")
				return
			}
		}(i)
	}
	wg.Wait()
	// Allow connections to close their end naturally
	time.Sleep(10 * time.Millisecond)
}
