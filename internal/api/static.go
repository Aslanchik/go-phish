package api

import (
	"io/fs"
	"net/http"
	"strings"

	webfs "github.com/aslanchik/go-phish/web"
)

// sub is the embedded dist/ directory, rooted so paths look like "index.html".
var distFS, _ = fs.Sub(webfs.StaticFS, "dist")

// handleSPA serves static assets from the embedded dist/ tree.
// If the requested path matches a real file it is served directly;
// everything else returns index.html so React Router can handle the route.
func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/")
	if name != "" {
		f, err := distFS.Open(name)
		if err == nil {
			stat, err2 := f.Stat()
			f.Close()
			if err2 == nil && !stat.IsDir() {
				http.FileServer(http.FS(distFS)).ServeHTTP(w, r)
				return
			}
		}
	}

	// SPA fallback: send index.html for all unmatched paths.
	data, err := fs.ReadFile(distFS, "index.html")
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}
