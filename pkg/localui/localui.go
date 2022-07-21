package localui

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/internal/ui_embed"
	"github.com/ghjm/connectopus/pkg/cpctl"
	"github.com/ghjm/connectopus/pkg/x/ssh_jwt"
	"golang.org/x/crypto/ssh/agent"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"time"
)

//go:generate go run gencli/gencli.go

type Opt func(*serveOpts)

type serveOpts struct {
	ctx         context.Context
	openBrowser bool
	localUIPort uint16
}

func WithContext(ctx context.Context) Opt {
	return func(so *serveOpts) {
		so.ctx = ctx
	}
}

func WithBrowser(wb bool) Opt {
	return func(so *serveOpts) {
		so.openBrowser = wb
	}
}

func WithLocalPort(port uint16) Opt {
	return func(so *serveOpts) {
		so.localUIPort = port
	}
}

func Serve(opts ...Opt) error {
	so := &serveOpts{
		ctx:         context.Background(),
		openBrowser: true,
		localUIPort: 26663,
	}
	for _, opt := range opts {
		opt(so)
	}

	mux := http.NewServeMux()
	mux.Handle("/", ui_embed.GetUIHandler())
	mux.HandleFunc("/api", ui_embed.PlaygroundHandler)

	var haveAgent bool
	var haveSocketFile bool

	localMux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pageSel := ""
		// Check if we have an SSH agent
		if !haveAgent {
			a, err := ssh_jwt.GetSSHAgent("")
			if err != nil {
				pageSel = "no-agent"
			} else {
				var keys []*agent.Key
				keys, err = a.List()
				if err != nil || len(keys) == 0 {
					pageSel = "no-agent"
				} else {
					haveAgent = true
				}
			}
		}

		// Check if we have a node socket
		if pageSel == "" && !haveSocketFile {
			socketFiles, err := cpctl.FindSockets("")
			if err != nil || len(socketFiles) != 1 {
				pageSel = "no-node"
			} else {
				var tokStr string
				tokStr, err = cpctl.GenToken("", "", "")
				if err != nil {
					pageSel = "unauthorized"
				} else {
					p := ui_embed.NewProxy(ui_embed.UnixSocketDialer(socketFiles[0]), ui_embed.UnixSocketDirector)
					mux.Handle("/query", p)
					mux.Handle("/proxy/", p)
					haveSocketFile = true
					v := r.URL.Query()
					v.Set("AuthToken", tokStr)
					r.URL.RawQuery = v.Encode()
					http.Redirect(w, r, r.URL.String(), http.StatusFound)
				}
			}
		}

		if pageSel == "" {
			r.Header.Del("page_select")
		} else {
			r.Header.Set("page_select", pageSel)
		}
		mux.ServeHTTP(w, r)
	})

	cookieSetterMux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		localMux.ServeHTTP(w, r)
	})

	sanitizedMux := ui_embed.GetUISanitizer(cookieSetterMux)

	srv := &http.Server{
		Handler:        sanitizedMux,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	li, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", so.localUIPort))
	if err != nil {
		return fmt.Errorf("tcp listen error: %s", err)
	}

	go func() {
		<-so.ctx.Done()
		_ = srv.Close()
		_ = li.Close()
	}()

	//url := fmt.Sprintf("http://localhost:%d/?AuthToken=%s", so.localUIPort, tokStr)
	url := fmt.Sprintf("http://localhost:%d", so.localUIPort)
	if so.openBrowser {
		fmt.Printf("Server started.  Launching browser.\n")
		err = openWebBrowser(url)
		if err != nil {
			return fmt.Errorf("error opening browser: %s", err)
		}
	} else {
		fmt.Printf("Server started on URL: %s\n", url)
	}

	return srv.Serve(li)
}

// openWebBrowser opens the default web browser to a given URL
func openWebBrowser(url string) error {
	var err error
	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		//nolint:goerr113
		err = fmt.Errorf("unsupported platform")
	}
	return err
}
