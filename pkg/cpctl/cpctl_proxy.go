package cpctl

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/internal/version"
	"github.com/ghjm/connectopus/pkg/proto"
	"net"
	"net/http"
	"net/http/httputil"
	"strconv"
	"strings"
)

const ProxyPortNo = 277

type proxy struct {
	httputil.ReverseProxy
}

func NewProxy(n proto.Netopus) *proxy {
	p := &proxy{}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
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
	p.Transport = transport
	director := func(req *http.Request) {
		urlParts := strings.Split(strings.Trim(req.URL.Path, "/"), "/")
		if len(urlParts) < 2 || urlParts[0] != "proxy" {
			req.URL = nil
			return
		}
		req.URL.Scheme = "http"
		req.URL.Host = fmt.Sprintf("%s:277", urlParts[1])
		req.URL.Path = "/" + strings.Join(urlParts[2:], "/")
		if _, ok := req.Header["User-Agent"]; !ok {
			req.Header.Set("User-Agent", fmt.Sprintf("connectopus/%s", version.Version()))
		}
	}
	p.Director = director
	return p
}
