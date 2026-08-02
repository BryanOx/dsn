package middleware

import (
	"net/http"
)

// SecurityHeadersMiddleware returns a middleware that adds security headers to all responses.
// If tlsEnabled is true, it also includes HSTS header.
func SecurityHeadersMiddleware(tlsEnabled bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Prevent content type sniffing
			w.Header().Set("X-Content-Type-Options", "nosniff")

			// Prevent clickjacking
			w.Header().Set("X-Frame-Options", "DENY")

			// XSS protection (legacy but still useful)
			w.Header().Set("X-XSS-Protection", "1; mode=block")

			// Prevent caching of sensitive responses
			w.Header().Set("Cache-Control", "no-store")

			// HSTS header - only when TLS is enabled
			if tlsEnabled {
				w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}

			next.ServeHTTP(w, r)
		})
	}
}
