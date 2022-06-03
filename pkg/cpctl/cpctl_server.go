package cpctl

import (
	"context"
	"fmt"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	log "github.com/sirupsen/logrus"
	"net"
	"net/http"
	"os"
	"path"
	"time"
)

func (r *Resolver) serve(ctx context.Context, li net.Listener) {
	mux := http.NewServeMux()
	mux.Handle("/", playground.Handler("GraphQL playground", "/query"))
	mux.Handle("/query", handler.NewDefaultServer(NewExecutableSchema(Config{Resolvers: r})))

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
	r.serve(ctx, li)
	return nil
}

func (r *Resolver) ServeHTTP(ctx context.Context, port int) error {
	li, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return err
	}
	r.serve(ctx, li)
	return nil
}
