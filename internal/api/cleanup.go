package api

import (
	"errors"
	"github.com/githubflyideas/vyos-sflow/internal/store"
	"net/http"
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
	writeJSON(w, http.StatusOK, map[string]any{"all": true, "estimate": all, "disk_bytes": u.DiskBytes, "oldest": u.Oldest})
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
