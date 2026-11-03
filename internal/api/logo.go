package api

import (
	"bytes"
	"io/fs"
	"net/http"
)

func (s *Server) logo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; img-src data:; sandbox")
	b, err := fs.ReadFile(s.Static, "logo.svg")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if r.URL.Query().Get("dark") == "1" {
		// the dark themes: light lettering
		b = bytes.Replace(b, []byte(`fill="#1b2432"`), []byte(`fill="#f2f2f7"`), 1)
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Write(b)
}
