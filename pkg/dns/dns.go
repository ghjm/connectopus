package dns

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/pkg/proto"
	"github.com/miekg/dns"
	log "github.com/sirupsen/logrus"
	"net"
	"strings"
	"time"
)

type Server struct {
	Domain     string
	PacketConn net.PacketConn
	Listener   net.Listener
	LookupName func(string) proto.IP
	LookupIP   func(proto.IP) string
}

func (s *Server) Run(ctx context.Context) error {
	handler := &dns.ServeMux{}
	handler.HandleFunc(s.Domain, func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)
		m.Compress = false
		if r.Opcode == dns.OpcodeQuery {
			for _, q := range m.Question {
				if q.Qtype == dns.TypeAAAA {
					qs := strings.TrimSuffix(q.Name, ".")
					qs = strings.TrimSuffix(qs, s.Domain)
					qs = strings.TrimSuffix(qs, ".")
					ip := s.LookupName(qs)
					if len(ip) > 0 {
						rr, err := dns.NewRR(fmt.Sprintf("%s AAAA %s", q.Name, ip.String()))
						if err == nil {
							m.Answer = append(m.Answer, rr)
							m.Authoritative = true
						}
					} else {
						m.SetRcode(r, dns.RcodeNameError)
					}
				}
			}
		}
		_ = w.WriteMsg(m)
	})
	handler.HandleFunc("ip6.arpa", func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)
		m.Compress = false
		if r.Opcode == dns.OpcodeQuery {
			for _, q := range m.Question {
				if q.Qtype == dns.TypePTR {
					qs := strings.TrimSuffix(q.Name, ".")
					qs = strings.TrimSuffix(qs, ".ip6.arpa")
					parts := strings.Split(qs, ".")
					var forwardAddr string
					good := true
					if len(parts) == 32 {
						for i := 31; i >= 0; i-- {
							p := parts[i]
							if len(p) != 1 {
								good = false
								break
							}
							forwardAddr += p
							if i%4 == 0 && i != 0 {
								forwardAddr += ":"
							}
						}
					}
					var ip proto.IP
					if good {
						ip = proto.ParseIP(forwardAddr)
					}
					if ip != "" {
						name := s.LookupIP(ip)
						if len(name) > 0 {
							rr, err := dns.NewRR(fmt.Sprintf("%s PTR %s", q.Name, name+"."+s.Domain+"."))
							if err == nil {
								m.Answer = append(m.Answer, rr)
								m.Authoritative = true
							}
						} else {
							m.SetRcode(r, dns.RcodeNameError)
						}
					}
				}
			}
		}
		_ = w.WriteMsg(m)
	})

	server := &dns.Server{
		Listener:     s.Listener,
		PacketConn:   s.PacketConn,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	errChan := make(chan error)
	go func() {
		err := server.ActivateAndServe()
		if err != nil && ctx.Err() == nil {
			errChan <- err
		}
	}()
	shutdown := func() {
		_ = s.Listener.Close()
		_ = s.PacketConn.Close()
		_ = server.Shutdown()
	}
	t := time.NewTimer(100 * time.Millisecond)
	select {
	case err := <-errChan:
		t.Stop()
		shutdown()
		return err
	case <-t.C:
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				shutdown()
				return
			case err := <-errChan:
				log.Warnf("dns error: %s", err)
			}
		}
	}()
	return nil
}
