package backend_dtls

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/pkg/backends"
	"github.com/ghjm/connectopus/pkg/config"
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

func RunDialer(ctx context.Context, pr backends.ProtocolRunner, destAddr net.IP, destPort uint16) error {
	addr := &net.UDPAddr{IP: destAddr, Port: int(destPort)}
	dtlsConfig := getDtlsConfig(ctx)
	go backends.RunDialer(ctx, pr, func() (backends.BackendConnection, error) {
		conn, err := dtls.DialWithContext(ctx, "udp", addr, dtlsConfig)
		if err != nil {
			return nil, err
		}
		go func() {
			<-ctx.Done()
			_ = conn.Close()
		}()
		return &dtlsBackend{
			mtu:  1400,
			conn: conn,
		}, nil
	})
	return nil
}

// RunDialerFromConfig runs a dialer from settings in a config.Params
func RunDialerFromConfig(ctx context.Context, pr backends.ProtocolRunner, params config.Params) error {
	ip, port, err := params.GetHostPort("peer")
	if err != nil {
		return fmt.Errorf("error parsing peer: %w", err)
	}
	return RunDialer(ctx, pr, ip, port)
}

func RunListener(ctx context.Context, pr backends.ProtocolRunner, listenPort uint16) (net.Addr, error) {
	addr := &net.UDPAddr{IP: net.ParseIP("0.0.0.0"), Port: int(listenPort)}
	dtlsConfig := getDtlsConfig(ctx)
	li, lerr := dtls.Listen("udp", addr, dtlsConfig)
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
		return &dtlsBackend{
			mtu:  1400,
			conn: conn.(*dtls.Conn),
		}, nil
	})
	return li.Addr(), nil
}

// RunListenerFromConfig runs a listener from settings in a config.Params
func RunListenerFromConfig(ctx context.Context, pr backends.ProtocolRunner, params config.Params) error {
	port, err := params.GetPort("port")
	if err != nil {
		return err
	}
	_, err = RunListener(ctx, pr, port)
	return err
}
