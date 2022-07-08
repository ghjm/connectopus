package backend_tcp

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"fmt"
	"github.com/ghjm/connectopus/pkg/backends"
	"github.com/ghjm/connectopus/pkg/config"
	log "github.com/sirupsen/logrus"
	"io"
	"net"
	"strconv"
	"time"
)

// implements BackendConnection
type tcpBackend struct {
	conn     net.Conn
	isServer bool
}

const MTU = 1500

func (b *tcpBackend) MTU() int {
	return MTU
}

func (b *tcpBackend) WriteMessage(data []byte) error {
	if len(data) > MTU {
		return backends.ErrExceedsMTU
	}
	if len(data) == 0 {
		return nil
	}
	packet := make([]byte, 2, len(data)+2)
	binary.BigEndian.PutUint16(packet, uint16(len(data)))
	packet = append(packet, data...)
	n, err := b.conn.Write(packet)
	if err != nil {
		return err
	}
	if n != len(data)+2 {
		return fmt.Errorf("expected to write %d bytes but only wrote %d", n, len(data)+2)
	}
	return nil
}

func (b *tcpBackend) ReadMessage() ([]byte, error) {
	lenBytes := make([]byte, 2)
	_, err := io.ReadFull(b.conn, lenBytes)
	if err != nil {
		return nil, err
	}
	msgLen := binary.BigEndian.Uint16(lenBytes)
	p := make([]byte, msgLen)
	_, err = io.ReadFull(b.conn, p)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (b *tcpBackend) Close() error {
	return b.conn.Close()
}

func (b *tcpBackend) SetReadDeadline(t time.Time) error {
	return b.conn.SetReadDeadline(t)
}

func (b *tcpBackend) SetWriteDeadline(t time.Time) error {
	return b.conn.SetWriteDeadline(t)
}

func (b *tcpBackend) IsServer() bool {
	return b.isServer
}

type Dialer struct {
	DestAddr net.IP
	DestPort uint16
	Cost     float32
	TLS      *tls.Config
}

func (d *Dialer) Run(ctx context.Context, pr backends.ProtocolRunner) error {
	go backends.RunDialer(ctx, pr, d.Cost, func() (backends.BackendConnection, error) {
		addr := net.JoinHostPort(d.DestAddr.String(), strconv.Itoa(int(d.DestPort)))
		log.Debugf("tcp dialing %s", addr)
		var conn net.Conn
		if d.TLS == nil {
			var err error
			conn, err = net.Dial("tcp", addr)
			if err != nil {
				return nil, err
			}
		} else {
			var err error
			conn, err = tls.Dial("tcp", addr, d.TLS)
			if err != nil {
				return nil, err
			}
		}
		go func() {
			<-ctx.Done()
			_ = conn.Close()
		}()
		return &tcpBackend{
			conn:     conn,
			isServer: false,
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
	var tlsEnabled bool
	tlsEnabled, err = params.GetBool("tls", false)
	if err != nil {
		return fmt.Errorf("error parsing tls: %w", err)
	}
	if tlsEnabled {
		d.TLS = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
		isv, ok := params["insecure_skip_verify"]
		if ok {
			d.TLS.InsecureSkipVerify, err = strconv.ParseBool(isv)
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
			d.TLS.RootCAs = pool
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
			d.TLS.Certificates = []tls.Certificate{cert}
		}
	}
	return d.Run(ctx, pr)
}

type Listener struct {
	ListenAddr net.IP
	ListenPort uint16
	Cost       float32
	TLS        *tls.Config
}

func (l *Listener) Run(ctx context.Context, pr backends.ProtocolRunner) (net.Addr, error) {
	var addr string
	if l.ListenAddr == nil {
		addr = fmt.Sprintf(":%d", l.ListenPort)
	} else {
		addr = net.JoinHostPort(l.ListenAddr.String(), strconv.Itoa(int(l.ListenPort)))
	}
	var li net.Listener
	if l.TLS == nil {
		var err error
		li, err = net.Listen("tcp", addr)
		if err != nil {
			return nil, err
		}
	} else {
		var err error
		li, err = tls.Listen("tcp", addr, l.TLS)
		if err != nil {
			return nil, err
		}
	}
	go func() {
		<-ctx.Done()
		_ = li.Close()
	}()
	go backends.RunListener(ctx, pr, l.Cost, func() (backends.BackendConnection, error) {
		conn, err := li.Accept()
		if err != nil {
			return nil, err
		}
		log.Debugf("tcp connection from %s", conn.RemoteAddr().String())
		go func() {
			<-ctx.Done()
			_ = conn.Close()
		}()
		return &tcpBackend{
			conn:     conn,
			isServer: true,
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
	var tlsEnabled bool
	tlsEnabled, err = params.GetBool("tls", false)
	if err != nil {
		return fmt.Errorf("error parsing tls: %w", err)
	}
	if tlsEnabled {
		l.TLS = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
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
		l.TLS.Certificates = []tls.Certificate{cert}
		var reqClientCert bool
		reqClientCert, err = params.GetBool("require_client_cert", false)
		if err != nil {
			return fmt.Errorf("error parsing require_client_cert: %w", err)
		}
		if reqClientCert {
			l.TLS.ClientAuth = tls.RequireAndVerifyClientCert
		}
		clientCA, ok := params["client_ca"]
		if ok {
			pool := x509.NewCertPool()
			ok := pool.AppendCertsFromPEM([]byte(clientCA))
			if !ok {
				return fmt.Errorf("failed to parse any certificataes from client_ca")
			}
			l.TLS.ClientCAs = pool
		}
	}
	_, err = l.Run(ctx, pr)
	return err
}
