package ui_embed

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
)

// UI routes list
//go:generate go run routegen/routegen.go

// UI embedded files
//go:embed embed
var uiFiles embed.FS

func GetUIHandler() http.Handler {
	// Create handler for the UI assets
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
			_, _ = w.Write(indexHTML)
		} else {
			subServer.ServeHTTP(w, r)
		}
	})
	return mux
}
