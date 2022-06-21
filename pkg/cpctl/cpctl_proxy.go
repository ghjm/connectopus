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
		netAddr := n.Status().NameToAddr[host]
		if netAddr == "" {
			return nil, fmt.Errorf("unknown node name")
		}
		netIP := net.ParseIP(netAddr)
		if netIP == nil {
			return nil, fmt.Errorf("invalid IP address")
		}
		var portNo int64
		portNo, err = strconv.ParseInt(port, 10, 16)
		if err != nil {
			return nil, err
		}
		if portNo != 277 {
			return nil, fmt.Errorf("invalid port number")
		}
		return n.DialContextTCP(ctx, netIP, uint16(portNo))
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
