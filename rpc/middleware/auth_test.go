package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthMiddleware_PassThroughWhenDisabled(t *testing.T) {
	// When no API key is configured, requests should pass through
	auth := NewAuthMiddleware("", "X-API-Key")

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	handler := auth.Middleware(next)
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if !nextCalled {
		t.Error("expected next handler to be called when auth is disabled")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
}

func TestAuthMiddleware_CorrectKeyPasses(t *testing.T) {
	auth := NewAuthMiddleware("secret-key", "X-API-Key")

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	handler := auth.Middleware(next)
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-API-Key", "secret-key")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if !nextCalled {
		t.Error("expected next handler to be called with correct API key")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
}

func TestAuthMiddleware_WrongKeyReturns403(t *testing.T) {
	auth := NewAuthMiddleware("secret-key", "X-API-Key")

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	})

	handler := auth.Middleware(next)
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-API-Key", "wrong-key")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if nextCalled {
		t.Error("expected next handler NOT to be called with wrong API key")
	}
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", rr.Code)
	}

	// Verify JSON-RPC error format in response
	var resp JSONRPCError
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.JSONRPC != "2.0" {
		t.Errorf("expected jsonrpc 2.0, got %s", resp.JSONRPC)
	}
	if resp.Error == nil || resp.Error.Code != -32001 {
		t.Errorf("expected error code -32001, got %v", resp.Error)
	}
	if resp.Error == nil || resp.Error.Message != "unauthorized" {
		t.Errorf("expected 'unauthorized' message, got %s", resp.Error.Message)
	}
}

func TestAuthMiddleware_MissingHeaderReturns403(t *testing.T) {
	auth := NewAuthMiddleware("secret-key", "X-API-Key")

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	})

	handler := auth.Middleware(next)
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if nextCalled {
		t.Error("expected next handler NOT to be called when header is missing")
	}
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", rr.Code)
	}
}

func TestAuthMiddleware_CustomHeaderName(t *testing.T) {
	auth := NewAuthMiddleware("secret-key", "Authorization")

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	handler := auth.Middleware(next)
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "secret-key")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if !nextCalled {
		t.Error("expected next handler to be called with custom header")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
}

func TestAuthMiddleware_EmptyApiKeyMeansDisabled(t *testing.T) {
	// Empty string should be treated as disabled
	auth := NewAuthMiddleware("", "X-API-Key")

	if auth.enabled {
		t.Error("expected auth to be disabled when API key is empty string")
	}
}

func TestAuthMiddleware_NonEmptyApiKeyMeansEnabled(t *testing.T) {
	auth := NewAuthMiddleware("some-key", "X-API-Key")

	if !auth.enabled {
		t.Error("expected auth to be enabled when API key is set")
	}
}
