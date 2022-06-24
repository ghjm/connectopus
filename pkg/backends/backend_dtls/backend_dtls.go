package backend_dtls

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"github.com/ghjm/connectopus/pkg/backends"
	"github.com/ghjm/connectopus/pkg/config"
	"github.com/pion/dtls/v2"
	"net"
	"strconv"
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

type Dialer struct {
	DestAddr           net.IP
	DestPort           uint16
	Cost               float32
	MTU                uint16
	PSK                string
	InsecureSkipVerify bool
	RootCAs            *x509.CertPool
	ClientCert         *tls.Certificate
}

func (d *Dialer) getDtlsConfig(ctx context.Context) *dtls.Config {
	var c *dtls.Config
	if d.PSK != "" {
		c = &dtls.Config{
			CipherSuites:         []dtls.CipherSuiteID{dtls.TLS_PSK_WITH_AES_128_GCM_SHA256},
			ExtendedMasterSecret: dtls.RequireExtendedMasterSecret,
			PSK: func(hint []byte) ([]byte, error) {
				return []byte(d.PSK), nil
			},
			PSKIdentityHint: []byte("Connectopus DTLS"),
			ConnectContextMaker: func() (context.Context, func()) {
				return context.WithTimeout(ctx, 30*time.Second)
			},
		}
	} else {
		c = &dtls.Config{
			InsecureSkipVerify: d.InsecureSkipVerify,
			RootCAs:            d.RootCAs,
			ConnectContextMaker: func() (context.Context, func()) {
				return context.WithTimeout(ctx, 30*time.Second)
			},
		}
		if d.ClientCert != nil {
			c.Certificates = []tls.Certificate{*d.ClientCert}
		}
	}
	return c
}

func (d *Dialer) Run(ctx context.Context, pr backends.ProtocolRunner) error {
	addr := &net.UDPAddr{IP: d.DestAddr, Port: int(d.DestPort)}
	dtlsConfig := d.getDtlsConfig(ctx)
	go backends.RunDialer(ctx, pr, d.Cost, func() (backends.BackendConnection, error) {
		conn, err := dtls.DialWithContext(ctx, "udp", addr, dtlsConfig)
		if err != nil {
			return nil, err
		}
		go func() {
			<-ctx.Done()
			_ = conn.Close()
		}()
		mtu := 1400
		if d.MTU > 0 {
			mtu = int(d.MTU)
		}
		return &dtlsBackend{
			mtu:  mtu,
			conn: conn,
		}, nil
	})
	return nil
}

// RunDialerFromConfig runs a dialer from settings in a config.Params
func RunDialerFromConfig(ctx context.Context, pr backends.ProtocolRunner, cost float32, params config.Params) error {
	ip, port, err := params.GetHostPort("peer")
	if err != nil {
		return fmt.Errorf("error parsing peer: %w", err)
	}
	d := &Dialer{
		DestAddr: ip,
		DestPort: port,
		Cost:     cost,
	}
	mtu, ok := params["mtu"]
	if ok {
		mtuInt, err := strconv.ParseInt(mtu, 10, 16)
		if err != nil {
			return fmt.Errorf("invalid MTU value")
		}
		d.MTU = uint16(mtuInt)
	}
	psk, ok := params["psk"]
	if ok {
		d.PSK = psk
		for _, p := range []string{"insecure_skip_verify", "root_ca", "client_cert", "client_cert_key"} {
			_, ok := params[p]
			if ok {
				return fmt.Errorf("cannot use certificate setting %s with PSK", p)
			}
		}
	} else {
		isv, ok := params["insecure_skip_verify"]
		if ok {
			d.InsecureSkipVerify, err = strconv.ParseBool(isv)
			if err != nil {
				return fmt.Errorf("error parsing insecure_skip_verify: %w", err)
			}
		}
		rootCA, ok := params["root_ca"]
		if ok {
			pool := x509.NewCertPool()
			ok := pool.AppendCertsFromPEM([]byte(rootCA))
			if !ok {
				return fmt.Errorf("failed to parse any certificataes from root_ca")
			}
			d.RootCAs = pool
		}
		clientCert, ok := params["client_cert"]
		if ok {
			clientKey, ok := params["client_key"]
			if !ok {
				return fmt.Errorf("must supply client_key with client_cert")
			}
			cert, err := tls.X509KeyPair([]byte(clientCert), []byte(clientKey))
			if err != nil {
				return fmt.Errorf("error parsing client certificate: %w", err)
			}
			d.ClientCert = &cert
		}
	}
	return d.Run(ctx, pr)
}

type Listener struct {
	ListenAddr        net.IP
	ListenPort        uint16
	Cost              float32
	MTU               uint16
	PSK               string
	RequireClientCert bool
	ClientCAs         *x509.CertPool
	Certificate       tls.Certificate
}

func (l *Listener) getDtlsConfig(ctx context.Context) *dtls.Config {
	var c *dtls.Config
	if l.PSK != "" {
		c = &dtls.Config{
			CipherSuites:         []dtls.CipherSuiteID{dtls.TLS_PSK_WITH_AES_128_GCM_SHA256},
			ExtendedMasterSecret: dtls.RequireExtendedMasterSecret,
			PSK: func(hint []byte) ([]byte, error) {
				return []byte(l.PSK), nil
			},
			PSKIdentityHint: []byte("Connectopus DTLS"),
			ConnectContextMaker: func() (context.Context, func()) {
				return context.WithTimeout(ctx, 30*time.Second)
			},
		}
	} else {
		c = &dtls.Config{
			Certificates: []tls.Certificate{l.Certificate},
			ClientCAs:    l.ClientCAs,
			ConnectContextMaker: func() (context.Context, func()) {
				return context.WithTimeout(ctx, 30*time.Second)
			},
		}
		if l.RequireClientCert {
			c.ClientAuth = dtls.RequireAndVerifyClientCert
		}
	}
	return c
}

func (l *Listener) Run(ctx context.Context, pr backends.ProtocolRunner) (net.Addr, error) {
	addr := &net.UDPAddr{IP: l.ListenAddr, Port: int(l.ListenPort)}
	dtlsConfig := l.getDtlsConfig(ctx)
	li, lerr := dtls.Listen("udp", addr, dtlsConfig)
	go func() {
		<-ctx.Done()
		_ = li.Close()
	}()
	if lerr != nil {
		return nil, lerr
	}
	go backends.RunListener(ctx, pr, l.Cost, func() (backends.BackendConnection, error) {
		conn, err := li.Accept()
		if err != nil {
			return nil, err
		}
		go func() {
			<-ctx.Done()
			_ = conn.Close()
		}()
		mtu := 1400
		if l.MTU > 0 {
			mtu = int(l.MTU)
		}
		return &dtlsBackend{
			mtu:  mtu,
			conn: conn.(*dtls.Conn),
		}, nil
	})
	return li.Addr(), nil
}

// RunListenerFromConfig runs a listener from settings in a config.Params
func RunListenerFromConfig(ctx context.Context, pr backends.ProtocolRunner, cost float32, params config.Params) error {
	listenPort, err := params.GetPort("port")
	if err != nil {
		return fmt.Errorf("invalid port number in listener: %w", err)
	}
	var listenIP net.IP
	if _, ok := params["listen_ip"]; ok {
		listenIP, err = params.GetIP("listen_ip")
		if err != nil {
			return fmt.Errorf("invalid listen_ip value: %w", err)
		}
	}
	l := &Listener{
		ListenAddr: listenIP,
		ListenPort: listenPort,
		Cost:       cost,
	}
	mtu, ok := params["mtu"]
	if ok {
		mtuInt, err := strconv.ParseInt(mtu, 10, 16)
		if err != nil {
			return fmt.Errorf("invalid MTU value")
		}
		l.MTU = uint16(mtuInt)
	}
	psk, ok := params["psk"]
	if ok {
		l.PSK = psk
		for _, p := range []string{"server_cert", "server_cert_key", "require_client_cert", "client_ca"} {
			_, ok := params[p]
			if ok {
				return fmt.Errorf("cannot use certificate setting %s with PSK", p)
			}
		}
	} else {
		serverCert, ok := params["server_cert"]
		if !ok {
			return fmt.Errorf("non-PSK listener requires server_cert")
		}
		serverKey, ok := params["server_key"]
		if !ok {
			return fmt.Errorf("non-PSK listener requires server_key")
		}
		cert, err := tls.X509KeyPair([]byte(serverCert), []byte(serverKey))
		if err != nil {
			return fmt.Errorf("error parsing server certificate: %w", err)
		}
		l.Certificate = cert
		rcc, ok := params["require_client_cert"]
		if ok {
			l.RequireClientCert, err = strconv.ParseBool(rcc)
			if err != nil {
				return fmt.Errorf("error parsing require_client_cert: %w", err)
			}
		}
		clientCA, ok := params["client_ca"]
		if ok {
			pool := x509.NewCertPool()
			ok := pool.AppendCertsFromPEM([]byte(clientCA))
			if !ok {
				return fmt.Errorf("failed to parse any certificataes from client_ca")
			}
			l.ClientCAs = pool
		}
	}
	_, err = l.Run(ctx, pr)
	return err
}
