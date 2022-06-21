package cpctl

import (
	"context"
	"fmt"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/ghjm/connectopus/internal/ui_embed"
	log "github.com/sirupsen/logrus"
	"net"
	"net/http"
	"os"
	"path"
	"time"
)

func (r *Resolver) runServer(ctx context.Context, li net.Listener, mux http.Handler) {
	srv := &http.Server{
		Handler:        mux,
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

func (r *Resolver) ServeUnix(ctx context.Context, socketFile string) error {
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
	return r.ServeAPI(ctx, li)
}

func (r *Resolver) ServeAPI(ctx context.Context, li net.Listener) error {
	mux := http.NewServeMux()
	mux.Handle("/query", handler.NewDefaultServer(NewExecutableSchema(Config{Resolvers: r})))

	p := NewProxy(r.N)
	mux.Handle("/proxy/", p)

	r.runServer(ctx, li, mux)
	return nil
}

func (r *Resolver) HandlePlayground(w http.ResponseWriter, req *http.Request) {
	p := req.URL.Query().Get("proxyTo")
	endpoint := "/query"
	title := "GraphQL Playground"
	if p != "" {
		endpoint = fmt.Sprintf("/proxy/%s/query", p)
		title = fmt.Sprintf("GraphQL Playground (%s)", p)
	}
	playground.Handler(title, endpoint)(w, req)
}

func (r *Resolver) ServeHTTP(ctx context.Context, li net.Listener) error {
	mux := http.NewServeMux()
	mux.Handle("/", ui_embed.GetUIHandler())
	mux.HandleFunc("/api", r.HandlePlayground)
	mux.Handle("/query", handler.NewDefaultServer(NewExecutableSchema(Config{Resolvers: r})))

	p := NewProxy(r.N)
	mux.Handle("/proxy/", p)

	r.runServer(ctx, li, mux)
	return nil
}
