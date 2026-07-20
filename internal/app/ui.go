package app

import (
	"bytes"
	"embed"
	"io/fs"
	"net/http"
	"strings"
	"time"
)

//go:embed all:ui/dist
var uiDist embed.FS

// spa serves the built management UI with SPA fallback semantics: the index
// page is never cached, content-hashed assets are immutable, and unknown
// paths fall back to the index so client-side routing survives refreshes.
type spa struct {
	index []byte
	fsys  fs.FS
}

func newSPA(fsys fs.FS) (*spa, error) {
	index, err := fs.ReadFile(fsys, "index.html")
	if err != nil {
		index = nil
	}
	return &spa{index: index, fsys: fsys}, nil
}

func embeddedSPA() *spa {
	sub, err := fs.Sub(uiDist, "ui/dist")
	if err != nil {
		return &spa{}
	}
	s, err := newSPA(sub)
	if err != nil {
		return &spa{}
	}
	return s
}

func (s *spa) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.index == nil {
			w.Header().Set("Cache-Control", "no-cache")
			http.Error(w, "management UI assets are not built; run npm run build in web/", http.StatusServiceUnavailable)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" || name == "index.html" {
			s.serveIndex(w)
			return
		}
		if strings.HasPrefix(name, "assets/") {
			data, err := fs.ReadFile(s.fsys, name)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
			return
		}
		s.serveIndex(w)
	})
}

func (s *spa) serveIndex(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(s.index)
}
