package api

import "net/http"

// SecurityHeaders adds browser hardening headers to every backend response
// (ASVS V12.4/V12.5, SPEC gap M3). The backend serves JSON only, so the
// Content-Security-Policy is the strictest possible: default-src 'none' blocks
// all resource loading and inline execution, and frame-ancestors 'none'
// prevents any framing (clickjacking). This closes reflected-content injection
// even though responses are JSON.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		// Disable all browser features (camera, mic, geo, etc.) policy-wide.
		h.Set("Permissions-Policy", "accelerometer=(), camera=(), geolocation=(), gyroscope=(), magnetometer=(), microphone=(), payment=(), usb=()")
		next.ServeHTTP(w, r)
	})
}

// isHTTPS reports whether the request arrived over TLS or was forwarded as
// HTTPS by a trusted reverse proxy.
func isHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return r.Header.Get("X-Forwarded-Proto") == "https"
}

// StrictTransportSecurity emits the HSTS header. It is applied whenever the
// request is HTTPS, independent of the SecureCookies flag (ASVS V12.5, SPEC
// gap M3): previously HSTS was coupled to SecureCookies, so a false flag
// silently dropped HSTS even over TLS. The SecureCookies flag remains the
// cookie-attribute toggle only. `preload` is included for public deployments;
// submission to the HSTS preload list is a separate manual step.
func StrictTransportSecurity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isHTTPS(r) {
			w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
		}
		next.ServeHTTP(w, r)
	})
}
