package localui

import (
	"context"
	"github.com/ghjm/connectopus/internal/ui_embed"
	"net"
	"net/http"
	"time"
)

//go:generate go run gencli/gencli.go

func Serve(ctx context.Context, li net.Listener, socketFilename string) error {
	mux := http.NewServeMux()
	mux.Handle("/", ui_embed.GetUIHandler())
	mux.HandleFunc("/api", ui_embed.PlaygroundHandler)
	p := ui_embed.NewProxy(ui_embed.UnixSocketDialer(socketFilename), ui_embed.UnixSocketDirector)
	mux.Handle("/query", p)
	mux.Handle("/proxy/", p)

	sanitizedMux := ui_embed.GetUISanitizer(mux.ServeHTTP)

	authMux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		tokReq := q.Get("AuthToken")
		if tokReq != "" {
			http.SetCookie(w, &http.Cookie{
				Name:     "AuthToken",
				Value:    tokReq,
				Secure:   r.TLS != nil,
				SameSite: http.SameSiteStrictMode,
			})
			u := r.URL
			q.Del("AuthToken")
			u.RawQuery = q.Encode()
			http.Redirect(w, r, u.String(), http.StatusFound)
			return
		}
		r.Header.Set("page-select", "unauthorized")
		sanitizedMux.ServeHTTP(w, r)
	})

	srv := &http.Server{
		Handler:        authMux,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()

	return srv.Serve(li)
}
