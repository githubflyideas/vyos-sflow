package api

import (
	"encoding/json"
	"errors"
	"github.com/githubflyideas/vyos-sflow/internal/store"
	"io"
	"log"
	"mime"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

// getCleanup offers only full flow-history deletion. Accounts, configuration,
// stream identity and unrelated legacy business data survive.
func (s *Server) getCleanup(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" {
		fail(w, errors.New("cleanup does not accept query parameters"))
		return
	}
	all := s.Store.PurgeSizeBefore(time.Time{})
	u := s.Store.Usage()
	exps, _ := s.Store.StoredExporters()
	writeJSON(w, http.StatusOK, map[string]any{"all": true, "estimate": all, "disk_bytes": u.DiskBytes, "oldest": u.Oldest, "exporters": exps})
}

func (s *Server) postCleanup(w http.ResponseWriter, r *http.Request) {
	if err := validateCleanupRequest(r); err != nil {
		fail(w, err)
		return
	}
	disk := s.Store.Usage().DiskBytes
	var done store.PurgeSize
	var err error
	clearHistory := func() error {
		if s.Pipe != nil {
			// Synchronize queue admission, buffered state and in-flight writes.
			done, err = s.Pipe.Purge(time.Time{})
		} else {
			done, err = s.Store.PurgeBefore(time.Time{})
		}
		return err
	}
	if s.Col != nil {
		// Take the collector lock first, matching Handle -> Submit. Holding a
		// pipeline lock while acquiring the collector would invert this order.
		err = s.Col.ClearHistory(clearHistory)
	} else {
		err = clearHistory()
	}
	if err != nil {
		fail(w, err)
		return
	}
	done.Bytes = max(0, disk-s.Store.Usage().DiskBytes)
	writeJSON(w, http.StatusOK, done)
}

// postPurgeExporter deletes all flow records of one device, for a device
// whose data should not be there (forged or broken exports). It asks for
// the device's address and an explicit confirmation.
func (s *Server) postPurgeExporter(w http.ResponseWriter, r *http.Request) {
	typ, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if typ != "application/json" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "application/json required"})
		return
	}
	var in struct {
		Exporter string `json:"exporter"`
		Confirm  bool   `json:"confirm"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&in); err != nil || !in.Confirm {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "exporter and confirm=true required"})
		return
	}
	a, err := netip.ParseAddr(strings.TrimSpace(in.Exporter))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_exporter"})
		return
	}
	disk := s.Store.Usage().DiskBytes
	var done store.PurgeSize
	if s.Pipe != nil {
		done, err = s.Pipe.PurgeExporter(a.Unmap().String())
	} else {
		done, err = s.Store.PurgeExporter(a.Unmap().String())
	}
	if err != nil {
		fail(w, err)
		return
	}
	done.Bytes = max(0, disk-s.Store.Usage().DiskBytes)
	log.Printf("cleanup: deleted %d flow records of device %s", done.Rows, a.Unmap())
	writeJSON(w, http.StatusOK, done)
}
