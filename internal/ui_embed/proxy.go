package ui_embed

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/internal/version"
	"github.com/ghjm/connectopus/pkg/proto"
	"github.com/sirupsen/logrus"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"strconv"
	"strings"
	"time"
)

type logrusWriter struct{}

func (w logrusWriter) Write(p []byte) (n int, err error) {
	sp := string(p)
	if !strings.Contains(sp, "proxy error: context canceled") {
		logrus.Warn(sp)
	}
	return len(p), nil
}

func NewProxy(
	dialer func(context.Context, string, string) (net.Conn, error),
	director func(*http.Request),
) *httputil.ReverseProxy {
	p := &httputil.ReverseProxy{
		ErrorLog: log.New(logrusWriter{}, "", 0),
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = dialer
	p.Transport = transport
	p.Director = director
	return p
}

const ProxyPortNo = 277

func OOBDialer(n proto.Netopus) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		if port != strconv.Itoa(ProxyPortNo) {
			return nil, fmt.Errorf("port must be %d", ProxyPortNo)
		}
		netAddr := n.Status().NameToAddr[host]
		if netAddr == "" {
			return nil, fmt.Errorf("unknown node name")
		}
		netIP := proto.ParseIP(netAddr)
		if netIP == "" {
			return nil, fmt.Errorf("invalid IP address")
		}
		return n.DialOOB(ctx, proto.OOBAddr{Host: netIP, Port: ProxyPortNo})
	}
}

func OOBDirector(req *http.Request) {
	urlParts := strings.Split(strings.Trim(req.URL.Path, "/"), "/")
	if len(urlParts) < 2 || urlParts[0] != "proxy" {
		req.URL = nil
		return
	}
	req.URL.Scheme = "http"
	req.URL.Host = fmt.Sprintf("%s:%d", urlParts[1], ProxyPortNo)
	req.URL.Path = "/" + strings.Join(urlParts[2:], "/")
	if _, ok := req.Header["User-Agent"]; !ok {
		req.Header.Set("User-Agent", fmt.Sprintf("connectopus/%s", version.Version()))
	}
}

func UnixSocketDialer(socketFile string) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		d := net.Dialer{
			Timeout: 30 * time.Second,
		}
		return d.DialContext(ctx, "unix", socketFile)
	}
}

func UnixSocketDirector(req *http.Request) {
	urlParts := strings.Split(strings.Trim(req.URL.Path, "/"), "/")
	if len(urlParts) == 0 || (urlParts[0] != "query" && urlParts[0] != "proxy") {
		req.URL = nil
		return
	}
	req.URL.Scheme = "http"
	req.URL.Host = "cpctl.sock"
	if _, ok := req.Header["User-Agent"]; !ok {
		req.Header.Set("User-Agent", fmt.Sprintf("connectopus/%s", version.Version()))
	}
}
