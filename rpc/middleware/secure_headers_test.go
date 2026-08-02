package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecurityHeadersMiddleware_WithoutTLS(t *testing.T) {
	middleware := SecurityHeadersMiddleware(false)

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	handler := middleware(next)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if !nextCalled {
		t.Error("expected next handler to be called")
	}

	// Check security headers
	if rr.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("expected X-Content-Type-Options to be nosniff")
	}
	if rr.Header().Get("X-Frame-Options") != "DENY" {
		t.Error("expected X-Frame-Options to be DENY")
	}
	if rr.Header().Get("X-XSS-Protection") != "1; mode=block" {
		t.Error("expected X-XSS-Protection to be 1; mode=block")
	}
	if rr.Header().Get("Cache-Control") != "no-store" {
		t.Error("expected Cache-Control to be no-store")
	}
	// HSTS should NOT be set without TLS
	if rr.Header().Get("Strict-Transport-Security") != "" {
		t.Error("expected HSTS header to NOT be set without TLS")
	}
}

func TestSecurityHeadersMiddleware_WithTLS(t *testing.T) {
	middleware := SecurityHeadersMiddleware(true)

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	handler := middleware(next)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if !nextCalled {
		t.Error("expected next handler to be called")
	}

	// Check HSTS is set when TLS is enabled
	if rr.Header().Get("Strict-Transport-Security") != "max-age=31536000; includeSubDomains" {
		t.Error("expected HSTS header to be set with TLS")
	}
}

func TestSecurityHeadersMiddleware_HeadersPresentOnErrorResponse(t *testing.T) {
	middleware := SecurityHeadersMiddleware(false)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	handler := middleware(next)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Security headers should still be present even on error responses
	if rr.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("expected X-Content-Type-Options on error response")
	}
	if rr.Header().Get("Cache-Control") != "no-store" {
		t.Error("expected Cache-Control on error response")
	}
}
