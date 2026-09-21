// Package operationscenter embeds the migration frontend into the Go binary.
// The existing Workbench remains served at /; this package owns only /next/.
package operationscenter

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strconv"
	"strings"
)

// Dist is the Vite production output. Keeping the output embedded means the
// browser still talks only to the existing same-origin Operations Center
// server; it never needs a separate frontend server in production.
//
//go:embed dist
var Dist embed.FS

// Handler serves the built application below /next/. Vite's base path makes
// asset URLs same-origin and the explicit fallback keeps the route usable on
// a direct navigation without introducing a client-side router contract.
func Handler() http.Handler {
	assets, err := fs.Sub(Dist, "dist")
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "frontend assets unavailable", http.StatusInternalServerError)
		})
	}
	indexHTML, err := fs.ReadFile(assets, "index.html")
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "frontend entrypoint unavailable", http.StatusInternalServerError)
		})
	}
	files := http.StripPrefix("/next", http.FileServer(http.FS(assets)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The browser router owns extensionless paths below /next. Serve the
		// embedded entrypoint for those client routes, while leaving assets and
		// other file-like requests to the exact static-file handler.
		clientPath := strings.TrimPrefix(r.URL.Path, "/next")
		if (r.Method == http.MethodGet || r.Method == http.MethodHead) &&
			clientPath != "" && clientPath != "/" &&
			!strings.HasPrefix(clientPath, "/assets/") &&
			path.Ext(clientPath) == "" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Content-Length", strconv.Itoa(len(indexHTML)))
			w.WriteHeader(http.StatusOK)
			if r.Method == http.MethodGet {
				_, _ = w.Write(indexHTML)
			}
			return
		}
		files.ServeHTTP(w, r)
	})
}
