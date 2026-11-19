package api

import (
	"encoding/json"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

// blocklist answers 409 unless there is a collector with a blocklist to
// change. Reading a flow file offline has no collector, so there is
// nothing to block and saying so beats a nil dereference.
func (s *Server) blocklist(w http.ResponseWriter) bool {
	if s.Col == nil || s.Col.Block == nil {
		apiError(w, http.StatusConflict, "no_collector")
		return false
	}
	return true
}

// postBlock stops reading datagrams from a source. It is the
// administrator's: it silences a device for everybody.
func (s *Server) postBlock(w http.ResponseWriter, r *http.Request) {
	if !s.blocklist(w) {
		return
	}
	var in struct {
		Exporter string `json:"exporter"`
		Note     string `json:"note"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil {
		apiError(w, http.StatusBadRequest, "bad_json")
		return
	}
	a, err := netip.ParseAddr(strings.TrimSpace(in.Exporter))
	if err != nil {
		apiError(w, http.StatusBadRequest, "bad_exporter")
		return
	}
	if err := s.Col.Block.Add(a, strings.TrimSpace(in.Note), time.Now()); err != nil {
		fail(w, err)
		return
	}
	audit(r, "source blocked", "exporter", a.Unmap(), "note", strings.TrimSpace(in.Note))
	writeJSON(w, http.StatusOK, map[string]any{"blocked": s.Col.Block.List()})
}

// deleteBlock starts reading from a source again, for the mis-click that
// silenced a switch somebody needed.
func (s *Server) deleteBlock(w http.ResponseWriter, r *http.Request) {
	if !s.blocklist(w) {
		return
	}
	a, err := netip.ParseAddr(strings.TrimSpace(r.URL.Query().Get("exporter")))
	if err != nil {
		apiError(w, http.StatusBadRequest, "bad_exporter")
		return
	}
	had, err := s.Col.Block.Remove(a)
	if err != nil {
		fail(w, err)
		return
	}
	if !had {
		apiError(w, http.StatusNotFound, "not_blocked")
		return
	}
	audit(r, "source unblocked", "exporter", a.Unmap())
	writeJSON(w, http.StatusOK, map[string]any{"blocked": s.Col.Block.List()})
}
