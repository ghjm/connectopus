package ui_embed

import (
	"embed"
	"fmt"
	"github.com/99designs/gqlgen/graphql/playground"
	"io/fs"
	"net/http"
	"strings"
)

// UI routes list
//go:generate go run routegen/routegen.go

// UI embedded files
//go:embed embed
var uiFiles embed.FS

// GetUISanitizer removes sensitive values from a request, to ensure clients can't do shenanigans with them
func GetUISanitizer(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Header.Set("page_select", "")
		r.Header.Set("page_extra", "")
		r.Header.Set("proxyTo", "")
		handler(w, r)
	}
}

func isIdentifierChar(c rune) bool {
	return c == '_' || c == '-' ||
		(c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9')
}

func GetUIHandler() http.Handler {
	subFiles, _ := fs.Sub(uiFiles, "embed/dist")
	subServer := http.FileServer(http.FS(subFiles))
	indexHTML, err := uiFiles.ReadFile("embed/dist/index.html")
	if err != nil {
		panic(fmt.Errorf("error reading embedded UI files: %w", err))
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		isRoute := false
		for _, rt := range uiRoutes {
			if r.URL.Path == rt {
				isRoute = true
				break
			}
		}
		if (r.URL.Path == "/") ||
			(r.URL.Path == "/index.html") ||
			isRoute {
			pageSel := r.Header.Get("page_select")
			if pageSel == "" {
				unauth := r.Header.Get("unauthorized")
				if unauth == "true" {
					pageSel = "unauthorized"
				}
			}
			for _, c := range pageSel {
				if !isIdentifierChar(c) {
					pageSel = "unauthorized"
					break
				}
			}
			dataValues := r.Header.Values("page_extra")
			if pageSel != "" {
				dataValues = append(dataValues, fmt.Sprintf("page_select=%s", pageSel))
			}
			if len(dataValues) == 0 {
				_, _ = w.Write(indexHTML)
			} else {
				replaceText := ""
				for _, v := range dataValues {
					splitV := strings.SplitN(v, "=", 2)
					if len(splitV) != 2 {
						continue
					}
					checkOK := true
					for _, s := range []string{splitV[0], splitV[1]} {
						for _, c := range s {
							if !isIdentifierChar(c) {
								checkOK = false
								break
							}
						}
					}
					if !checkOK {
						continue
					}
					replaceText += fmt.Sprintf("\"%s\": \"%s\", ", splitV[0], splitV[1])
				}
				repHTML := strings.Replace(string(indexHTML), "//$!&@SERVER_DATA@&!$//", replaceText, 1)
				_, _ = w.Write([]byte(repHTML))
			}
		} else {
			subServer.ServeHTTP(w, r)
		}
	})
	return mux
}

func PlaygroundHandler(w http.ResponseWriter, req *http.Request) {
	p := req.URL.Query().Get("proxyTo")
	endpoint := "/query"
	title := "GraphQL Playground"
	if p != "" {
		endpoint = fmt.Sprintf("/proxy/%s/query", p)
		title = fmt.Sprintf("GraphQL Playground (%s)", p)
	}
	playground.Handler(title, endpoint)(w, req)
}
