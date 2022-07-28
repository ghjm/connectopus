package localui

import (
	"context"
	"fmt"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/ghjm/connectopus/internal/ui_embed"
	"github.com/ghjm/connectopus/pkg/cpctl"
	"github.com/ghjm/connectopus/pkg/x/ssh_jwt"
	"github.com/ghjm/connectopus/pkg/x/syncro"
	"github.com/google/uuid"
	"golang.org/x/crypto/ssh/agent"
	"net"
	"net/http"
	"os/exec"
	"path"
	"runtime"
	"strings"
	"time"
)

//go:generate go run gencli/gencli.go

type Opt func(*serveOpts)

type serveOpts struct {
	ctx         context.Context
	openBrowser bool
	localUIPort uint16
	socketNode  string
	token       string
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

func WithNode(node string) Opt {
	return func(so *serveOpts) {
		so.socketNode = node
	}
}

func WithToken(tok string) Opt {
	return func(so *serveOpts) {
		if tok == "" {
			tok = strings.ReplaceAll(uuid.New().String(), "-", "")
		}
		so.token = tok
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

	preferredSocketNode := syncro.NewVar[string](so.socketNode)
	r := Resolver{
		AvailableNodesFunc: func() ([]string, error) {
			socketFiles, err := cpctl.FindSockets(preferredSocketNode.Get())
			if err != nil {
				return nil, err
			}
			nodes := make([]string, 0, len(socketFiles))
			for _, sf := range socketFiles {
				nodes = append(nodes, path.Base(path.Dir(sf)))
			}
			return nodes, nil
		},
		PreferredSocketFunc: func(ps string) error {
			preferredSocketNode.Set(ps)
			return nil
		},
	}

	mux := http.NewServeMux()
	mux.Handle("/", ui_embed.GetUIHandler())
	mux.HandleFunc("/api", ui_embed.PlaygroundHandler)
	mux.Handle("/localquery", handler.NewDefaultServer(NewExecutableSchema(Config{Resolvers: &r})))

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
		if !haveSocketFile {
			socketFiles, err := cpctl.FindSockets(preferredSocketNode.Get())
			if err != nil || len(socketFiles) == 0 {
				pageSel = "no-node"
			} else if len(socketFiles) > 1 {
				pageSel = "multi-node"
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
		r.Header.Set("page_extra", "localui=true")
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

	var localAuthMux http.Handler
	if so.token == "" {
		localAuthMux = sanitizedMux
	} else {
		localAuthMux = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			tokReq := q.Get("LocalToken")
			if tokReq != "" {
				http.SetCookie(w, &http.Cookie{
					Name:     "LocalToken",
					Value:    tokReq,
					Secure:   r.TLS != nil,
					SameSite: http.SameSiteStrictMode,
				})
				u := r.URL
				q.Del("LocalToken")
				u.RawQuery = q.Encode()
				http.Redirect(w, r, u.String(), http.StatusFound)
				return
			}
			c, err := r.Cookie("LocalToken")
			if err == nil && c != nil && c.Value == so.token {
				sanitizedMux.ServeHTTP(w, r)
			} else {
				w.WriteHeader(401)
				_, _ = w.Write([]byte("Unauthorized"))
				return
			}
		})
	}

	srv := &http.Server{
		Handler:           localAuthMux,
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      10 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	li, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", so.localUIPort))
	if err != nil {
		return fmt.Errorf("tcp listen error: %w", err)
	}

	srvCtx, srvCancel := context.WithCancel(so.ctx)
	defer srvCancel()

	go func() {
		<-srvCtx.Done()
		_ = srv.Close()
		_ = li.Close()
	}()

	var urlTok string
	if so.token != "" {
		urlTok = fmt.Sprintf("/?LocalToken=%s", so.token)
	}
	url := fmt.Sprintf("http://localhost:%d%s", so.localUIPort, urlTok)
	if so.openBrowser {
		fmt.Printf("Server started.  Launching browser.\n")
		err = OpenWebBrowser(url)
		if err != nil {
			return fmt.Errorf("error opening browser: %w", err)
		}
	} else {
		fmt.Printf("Server started.  URL: %s\n", url)
	}

	return srv.Serve(li)
}

// OpenWebBrowser opens the default web browser to a given URL
func OpenWebBrowser(url string) error {
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
