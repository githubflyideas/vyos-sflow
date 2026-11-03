package api

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCleanupRequiresExplicitAllConfirmation(t *testing.T) {
	for _, body := range []string{"", `{}`, `{"all":true}`, `{"confirm":true}`, `{"all":false,"confirm":true}`, `{"all":true,"confirm":false}`, `{"all":"true","confirm":true}`, `{"days":0}`, `{"all":true,"confirm":true,"before":1}`, `{"all":true,"confirm":true,"cutoff":1}`, `{"all":true,"confirm":true,"days":0}`, `{"all":true,"confirm":true} {}`, `{"all":true,"all":true,"confirm":true}`} {
		r := httptest.NewRequest("POST", "http://localhost/api/cleanup", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if err := validateCleanupRequest(r); err == nil {
			t.Errorf("accepted %s", body)
		}
	}
	for _, query := range []string{"before=1", "cutoff=1", "days=0", "all=true"} {
		r := httptest.NewRequest("POST", "http://localhost/api/cleanup?"+query, strings.NewReader(`{"all":true,"confirm":true}`))
		r.Header.Set("Content-Type", "application/json")
		if err := validateCleanupRequest(r); err == nil {
			t.Errorf("accepted query %s", query)
		}
	}
	r := httptest.NewRequest("POST", "http://localhost/api/cleanup", strings.NewReader(`{"all":true,"confirm":true}`))
	r.Header.Set("Content-Type", "application/json")
	if err := validateCleanupRequest(r); err != nil {
		t.Fatal(err)
	}
}
func TestCleanupRejectsCrossSiteRequest(t *testing.T) {
	for _, tc := range []struct{ typ, origin, fetch string }{
		{"text/plain", "", ""}, {"application/x-www-form-urlencoded", "", ""},
		{"application/json", "http://evil.example", ""}, {"application/json", "", "cross-site"},
	} {
		r := httptest.NewRequest("POST", "http://localhost/api/cleanup", strings.NewReader(`{"all":true,"confirm":true}`))
		r.Header.Set("Content-Type", tc.typ)
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("Sec-Fetch-Site", tc.fetch)
		if err := validateCleanupRequest(r); err == nil {
			t.Errorf("accepted cross-site/simple request %+v", tc)
		}
	}
}
