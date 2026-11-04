package api

import (
	"fmt"
	"net/http/httptest"
	"net/url"
	"testing"
	"testing/fstest"
	"time"
)

func TestRemovedProductRoutes(t *testing.T) {
	s := &Server{Static: fstest.MapFS{"index.html": {Data: []byte("SPA")}}, Users: map[string]string{"admin": "secret"}}
	h := s.Handler()
	for _, path := range []string{"report", "sankey", "rings", "geolines", "ifaces", "recon", "threats", "threats/lists", "findings", "racks", "racks/device", "racks/iface", "inventory", "iface", "geo", "geo/dbip", "logo"} {
		for _, method := range []string{"GET", "POST", "DELETE"} {
			req := httptest.NewRequest(method, "/api/"+path, nil)
			req.SetBasicAuth("admin", "secret")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			if w.Code != 404 && w.Code != 405 {
				t.Errorf("%s %s: %d %s", method, path, w.Code, w.Body)
			}
		}
	}
}

func TestHistoryBoundary(t *testing.T) {
	now := time.Now().UTC()
	explicit := func(a, b time.Time) string { return fmt.Sprintf("from=%d&to=%d", a.UnixMilli(), b.UnixMilli()) }
	for _, params := range []string{"range=30d", "range=8d", "range=garbage", "from=1", "to=1", explicit(now.Add(-8*24*time.Hour), now), explicit(now.Add(-9*24*time.Hour), now.Add(-8*24*time.Hour))} {
		if _, err := parseQuery(httptest.NewRequest("GET", "/api/overview?"+params, nil)); err == nil {
			t.Errorf("accepted %s", params)
		}
	}
	for _, span := range []string{"5m", "15m", "1h", "6h", "24h", "48h"} {
		before := time.Now().UTC().Truncate(time.Millisecond)
		q, err := parseQuery(httptest.NewRequest("GET", "/api/overview?range="+span, nil))
		if err != nil {
			t.Fatal(err)
		}
		if q.To.Sub(q.From) > 48*time.Hour || q.From.Before(before.Add(-48*time.Hour)) {
			t.Errorf("unbounded %s: %+v", span, q)
		}
	}
	q, err := parseQuery(httptest.NewRequest("GET", "/api/overview", nil))
	if err != nil || q.To.Sub(q.From) != time.Hour {
		t.Fatalf("default %+v %v", q, err)
	}
	for _, field := range []string{"country", "asn", "org", "threat", "segment", "iface", "iface_in", "client", "server", "vlan", "encap"} {
		f := url.QueryEscape(fmt.Sprintf(`[{"f":%q,"v":"anything"}]`, field))
		if _, err := parseQuery(httptest.NewRequest("GET", "/api/overview?f="+f, nil)); err == nil {
			t.Errorf("accepted filter %s", field)
		}
	}
}
