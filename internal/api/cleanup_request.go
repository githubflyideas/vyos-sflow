package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
)

// Only explicit full-history deletion is supported. Strict parsing prevents
// legacy partial-cleanup requests from silently becoming full deletion.
func validateCleanupRequest(r *http.Request) error {
	if r.URL.RawQuery != "" {
		return fmt.Errorf("cleanup does not accept query parameters")
	}
	typ, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || typ != "application/json" {
		return fmt.Errorf("cleanup requires application/json")
	}
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return fmt.Errorf("cross-site cleanup is forbidden")
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host != r.Host || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
			return fmt.Errorf("cross-origin cleanup is forbidden")
		}
	}
	b, err := io.ReadAll(io.LimitReader(r.Body, 1025))
	if err != nil || len(b) > 1024 {
		return fmt.Errorf("invalid cleanup request body")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return fmt.Errorf("cleanup requires all=true and confirm=true")
	}
	seen := map[string]bool{}
	for d.More() {
		token, err = d.Token()
		if err != nil {
			return fmt.Errorf("invalid cleanup request")
		}
		key, ok := token.(string)
		if !ok || (key != "all" && key != "confirm") || seen[key] {
			return fmt.Errorf("unknown or duplicate cleanup field")
		}
		var value bool
		if err = d.Decode(&value); err != nil || !value {
			return fmt.Errorf("cleanup requires all=true and confirm=true")
		}
		seen[key] = true
	}
	if _, err = d.Token(); err != nil {
		return fmt.Errorf("invalid cleanup request")
	}
	if !seen["all"] || !seen["confirm"] {
		return fmt.Errorf("cleanup requires all=true and confirm=true")
	}
	if _, err = d.Token(); err != io.EOF {
		return fmt.Errorf("trailing cleanup request data")
	}
	return nil
}
