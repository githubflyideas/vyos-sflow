package api

import "net/http"

// ContentSecurityPolicy matches the embedded UI: only same-origin scripts
// (index.html loads theme.js and app.js, no inline script or event handler
// attributes), same-origin fetches, same-origin or data: images (the logo,
// manual images and the inline favicon). Inline styles stay allowed because
// the UI sets style attributes in generated markup (bars, swatches, chart
// tooltips). frame-ancestors 'none' stops the dashboard being framed.
const ContentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; font-src 'self'; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'"

// securityHeaders sets defensive headers on every response. Handlers may
// still replace them with stricter values (the logo sets its own sandboxed
// policy).
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", ContentSecurityPolicy)
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}
