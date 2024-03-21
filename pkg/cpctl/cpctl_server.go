package cpctl

import (
	"context"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/ghjm/connectopus/internal/ui_embed"
	"github.com/ghjm/golib/pkg/file_cleaner"
	"github.com/ghjm/golib/pkg/ssh_jwt"
	"github.com/golang-jwt/jwt/v4"
	log "github.com/sirupsen/logrus"
	"net"
	"net/http"
	"os"
	"path"
	"time"
)

type Server struct {
	Resolver
	SigningMethod jwt.SigningMethod
}

func (s *Server) runServer(ctx context.Context, li net.Listener, mux http.Handler, auth bool) {
	serverMux := mux
	if auth {
		var authKeys []string
		cfg := s.GetConfig()
		if cfg != nil {
			for _, k := range cfg.Global.AuthorizedKeys {
				authKeys = append(authKeys, k.String())
			}
		}
		serverMux = &ssh_jwt.Handler{
			AuthorizedKeys: authKeys,
			SigningMethod:  s.SigningMethod,
			Handler:        mux,
			StrictPaths:    []string{"/query", "/proxy"},
		}
	}
	srv := &http.Server{
		Handler:           serverMux,
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      10 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	go func() {
		err := srv.Serve(li)
		if err != nil && err != http.ErrServerClosed && ctx.Err() == nil {
			log.Errorf("cpctl socket server failed: %s", err)
		}
	}()

	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
}

func (s *Server) ServeUnix(ctx context.Context, socketFile string) (net.Listener, error) {
	err := os.MkdirAll(path.Dir(socketFile), 0700)
	if err != nil {
		return nil, err
	}
	err = os.Remove(socketFile)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	var li net.Listener
	li, err = net.Listen("unix", socketFile)
	if err != nil {
		return nil, err
	}
	delHandle := file_cleaner.DeleteOnExit(socketFile)
	go func() {
		<-ctx.Done()
		_ = delHandle.DeleteNow()
	}()
	err = os.Chmod(socketFile, 0600)
	if err != nil {
		_ = li.Close()
		return nil, err
	}
	err = s.ServeHTTP(ctx, li)
	if err != nil {
		_ = li.Close()
		return nil, err
	}
	return li, nil
}

func (s *Server) ServeHTTP(ctx context.Context, li net.Listener) error {
	mux := http.NewServeMux()
	mux.Handle("/", ui_embed.GetUIHandler())
	mux.HandleFunc("/api", ui_embed.PlaygroundHandler)
	mux.Handle("/query", handler.NewDefaultServer(NewExecutableSchema(Config{Resolvers: s})))
	p := ui_embed.NewProxy(ui_embed.OOBDialer(s.GetNetopus()), ui_embed.OOBDirector)
	mux.Handle("/proxy/", p)
	sanitizedMux := ui_embed.GetUISanitizer(mux.ServeHTTP)
	s.runServer(ctx, li, sanitizedMux, true)
	return nil
}
