// Package operationscenter embeds the Operations Center frontend into the Go
// binary. React is canonical at /; /next/ is a compatibility redirect.
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

func Handler() http.Handler {
	return handlerAtRoot()
}

// CompatibilityHandler redirects the historical /next/ prefix to the
// canonical root without creating a second SPA identity.
func CompatibilityHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		target := strings.TrimPrefix(r.URL.Path, "/next")
		if target == "" {
			target = "/"
		}
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, target, http.StatusPermanentRedirect)
	})
}

func handlerAtRoot() http.Handler {
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
	files := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// The browser router owns extensionless paths at root. Serve the
		// embedded entrypoint for those client routes, while leaving assets and
		// other file-like requests to the exact static-file handler.
		clientPath := r.URL.Path
		if clientPath != "" && clientPath != "/" &&
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
