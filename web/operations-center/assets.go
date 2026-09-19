// Package operationscenter embeds the migration frontend into the Go binary.
// The existing Workbench remains served at /; this package owns only /next/.
package operationscenter

import (
	"embed"
	"io/fs"
	"net/http"
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
	files := http.StripPrefix("/next", http.FileServer(http.FS(assets)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		files.ServeHTTP(w, r)
	})
}
