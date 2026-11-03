package api

import "net/http"

// data serves only the live sFlow database. Reject old offline links rather
// than silently showing unrelated live data for an unsupported selector.
func (s *Server) data(h func(*Server, http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("ds") != "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "offline analysis is not available"})
			return
		}

		h(s, w, r)
	}
}
