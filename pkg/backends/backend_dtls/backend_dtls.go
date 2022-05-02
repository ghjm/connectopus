package backend_dtls

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/pkg/backends"
	"github.com/pion/dtls/v2"
	"net"
	"time"
)

// implements BackendConnection
type dtlsBackend struct {
	mtu  int
	conn *dtls.Conn
}

func (b *dtlsBackend) MTU() int {
	return b.mtu
}

func (b *dtlsBackend) WriteMessage(data []byte) error {
	if len(data) > b.mtu {
		return backends.ErrExceedsMDU
	}
	n, err := b.conn.Write(data)
	if err != nil {
		return err
	}
	if n != len(data) {
		return fmt.Errorf("expected to write %d bytes but only wrote %d", n, len(data))
	}
	return nil
}

func (b *dtlsBackend) ReadMessage() ([]byte, error) {
	p := make([]byte, b.mtu)
	n, err := b.conn.Read(p)
	return p[:n], err
}

func (b *dtlsBackend) Close() error {
	return b.conn.Close()
}

func (b *dtlsBackend) SetReadDeadline(t time.Time) error {
	return b.conn.SetReadDeadline(t)
}

func (b *dtlsBackend) SetWriteDeadline(t time.Time) error {
	return b.conn.SetWriteDeadline(t)
}

func getDtlsConfig(ctx context.Context) *dtls.Config {
	// TODO: certificates
	return &dtls.Config{
		PSK: func(hint []byte) ([]byte, error) {
			return []byte{0xAB, 0xC1, 0x23}, nil
		},
		PSKIdentityHint:      []byte("Connectopus DTLS"),
		CipherSuites:         []dtls.CipherSuiteID{dtls.TLS_PSK_WITH_AES_128_GCM_SHA256},
		ExtendedMasterSecret: dtls.RequireExtendedMasterSecret,
		ConnectContextMaker: func() (context.Context, func()) {
			return context.WithTimeout(ctx, 30*time.Second)
		},
	}
}

func RunDialer(ctx context.Context, pr backends.ProtocolRunner, destAddr net.IP, destPort int) error {
	addr := &net.UDPAddr{IP: destAddr, Port: destPort}
	config := getDtlsConfig(ctx)
	go backends.RunDialer(ctx, pr, func() (backends.BackendConnection, error) {
		conn, err := dtls.DialWithContext(ctx, "udp", addr, config)
		if err != nil {
			return nil, err
		}
		go func() {
			<-ctx.Done()
			_ = conn.Close()
		}()
		// TODO: get MTU from transport layer
		return &dtlsBackend{
			mtu:  1200,
			conn: conn,
		}, nil
	})
	return nil
}

func RunListener(ctx context.Context, pr backends.ProtocolRunner, listenPort int) (net.Addr, error) {
	addr := &net.UDPAddr{IP: net.ParseIP("0.0.0.0"), Port: listenPort}
	config := getDtlsConfig(ctx)
	li, lerr := dtls.Listen("udp", addr, config)
	go func() {
		<-ctx.Done()
		_ = li.Close()
	}()
	if lerr != nil {
		return nil, lerr
	}
	go backends.RunListener(ctx, pr, func() (backends.BackendConnection, error) {
		conn, err := li.Accept()
		if err != nil {
			return nil, err
		}
		go func() {
			<-ctx.Done()
			_ = conn.Close()
		}()
		// TODO: get MTU from transport
		return &dtlsBackend{
			mtu:  1200,
			conn: conn.(*dtls.Conn),
		}, nil
	})
	return li.Addr(), nil
}
