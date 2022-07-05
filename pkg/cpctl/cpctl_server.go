package cpctl

import (
	"context"
	"fmt"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/ghjm/connectopus/internal/ui_embed"
	"github.com/ghjm/connectopus/pkg/x/ssh_jwt"
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
		for _, k := range s.C.Global.AuthorizedKeys {
			authKeys = append(authKeys, k.String())
		}
		serverMux = &ssh_jwt.Handler{
			AuthorizedKeys: authKeys,
			SigningMethod:  s.SigningMethod,
			Handler:        mux,
		}
	}
	srv := &http.Server{
		Handler:        serverMux,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	go func() {
		err := srv.Serve(li)
		if err != nil && err != http.ErrServerClosed {
			log.Errorf("cpctl socket server failed: %s", err)
		}
	}()

	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
}

func (s *Server) ServeUnix(ctx context.Context, socketFile string) error {
	err := os.MkdirAll(path.Dir(socketFile), 0700)
	if err != nil {
		return err
	}
	err = os.Remove(socketFile)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	var li net.Listener
	li, err = net.Listen("unix", socketFile)
	if err != nil {
		return err
	}
	err = os.Chmod(socketFile, 0600)
	if err != nil {
		return err
	}
	return s.ServeHTTP(ctx, li, false)
}

func (s *Server) HandlePlayground(w http.ResponseWriter, req *http.Request) {
	p := req.URL.Query().Get("proxyTo")
	endpoint := "/query"
	title := "GraphQL Playground"
	if p != "" {
		endpoint = fmt.Sprintf("/proxy/%s/query", p)
		title = fmt.Sprintf("GraphQL Playground (%s)", p)
	}
	playground.Handler(title, endpoint)(w, req)
}

func (s *Server) ServeHTTP(ctx context.Context, li net.Listener, auth bool) error {
	mux := http.NewServeMux()
	mux.Handle("/", ui_embed.GetUIHandler())
	mux.HandleFunc("/api", s.HandlePlayground)
	mux.Handle("/query", handler.NewDefaultServer(NewExecutableSchema(Config{Resolvers: s})))

	p := NewProxy(s.N)
	mux.Handle("/proxy/", p)

	s.runServer(ctx, li, mux, auth)
	return nil
}
