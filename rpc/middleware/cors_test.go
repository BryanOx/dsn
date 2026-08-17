package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCORSDefaultReject(t *testing.T) {
	// Empty AllowedOrigins → no CORS header in response
	handler := CORS(CORSConfig{AllowedOrigins: []string{}})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", "https://evil.com")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Assert NO Access-Control-Allow-Origin header
	if origin := rec.Header().Get("Access-Control-Allow-Origin"); origin != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty (no CORS header)", origin)
	}
}

func TestCORSDefaultRejectNoOrigin(t *testing.T) {
	// Empty AllowedOrigins with no Origin header → no CORS header, but next handler is called
	called := false
	handler := CORS(CORSConfig{AllowedOrigins: []string{}})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("expected next handler to be called when no Origin header")
	}
	if origin := rec.Header().Get("Access-Control-Allow-Origin"); origin != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty", origin)
	}
}

func TestCORSConfigured(t *testing.T) {
	// AllowedOrigins=["https://app.dsn.io"] → matching origin allowed
	handler := CORS(CORSConfig{
		AllowedOrigins: []string{"https://app.dsn.io"},
		AllowedMethods: []string{"POST", "OPTIONS"},
		AllowedHeaders: []string{"Content-Type"},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/", nil)
	req.Header.Set("Origin", "https://app.dsn.io")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	origin := rec.Header().Get("Access-Control-Allow-Origin")
	if origin != "https://app.dsn.io" {
		t.Errorf("Access-Control-Allow-Origin = %q, want https://app.dsn.io", origin)
	}
}

func TestCORSConfiguredRejectMismatch(t *testing.T) {
	// AllowedOrigins=["https://app.dsn.io"] → different origin rejected
	handler := CORS(CORSConfig{
		AllowedOrigins: []string{"https://app.dsn.io"},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/", nil)
	req.Header.Set("Origin", "https://evil.com")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if origin := rec.Header().Get("Access-Control-Allow-Origin"); origin != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty (mismatched origin)", origin)
	}
}

func TestCORSPreflightAllowed(t *testing.T) {
	handler := CORS(CORSConfig{
		AllowedOrigins: []string{"https://app.dsn.io"},
		AllowedMethods: []string{"POST", "OPTIONS"},
		AllowedHeaders: []string{"Content-Type"},
		MaxAge:         86400,
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("OPTIONS", "/", nil)
	req.Header.Set("Origin", "https://app.dsn.io")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("preflight status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	origin := rec.Header().Get("Access-Control-Allow-Origin")
	if origin != "https://app.dsn.io" {
		t.Errorf("preflight Access-Control-Allow-Origin = %q, want https://app.dsn.io", origin)
	}
	methods := rec.Header().Get("Access-Control-Allow-Methods")
	if !strings.Contains(methods, "POST") {
		t.Errorf("preflight Access-Control-Allow-Methods = %q, want POST", methods)
	}
}

func TestCORSPreflightRejected(t *testing.T) {
	handler := CORS(CORSConfig{
		AllowedOrigins: []string{"https://app.dsn.io"},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("OPTIONS", "/", nil)
	req.Header.Set("Origin", "https://evil.com")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("preflight status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if origin := rec.Header().Get("Access-Control-Allow-Origin"); origin != "" {
		t.Errorf("preflight Access-Control-Allow-Origin = %q, want empty (rejected)", origin)
	}
}

func TestCORSMaxAgeHeader(t *testing.T) {
	// Verify MaxAge is formatted as a decimal string, not a rune
	handler := CORS(CORSConfig{
		AllowedOrigins: []string{"https://app.dsn.io"},
		AllowedMethods: []string{"POST"},
		MaxAge:         86400,
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("OPTIONS", "/", nil)
	req.Header.Set("Origin", "https://app.dsn.io")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	maxAge := rec.Header().Get("Access-Control-Max-Age")
	if maxAge != "86400" {
		t.Errorf("Access-Control-Max-Age = %q, want %q", maxAge, "86400")
	}
}

func TestCORSWildcard(t *testing.T) {
	handler := CORS(CORSConfig{
		AllowedOrigins: []string{"*"},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/", nil)
	req.Header.Set("Origin", "https://any-origin.com")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	origin := rec.Header().Get("Access-Control-Allow-Origin")
	if origin != "https://any-origin.com" {
		t.Errorf("Access-Control-Allow-Origin = %q, want https://any-origin.com", origin)
	}
}

func TestCORSNoOriginHeader(t *testing.T) {
	// Request without Origin header (e.g. same-origin) should pass through
	called := false
	handler := CORS(CORSConfig{
		AllowedOrigins: []string{"https://app.dsn.io"},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("expected next handler to be called when no Origin header")
	}
	// No CORS header needed for same-origin
	if origin := rec.Header().Get("Access-Control-Allow-Origin"); origin != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty (no origin header)", origin)
	}
}
