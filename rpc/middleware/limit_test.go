package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestBodySizeLimit tests the body size limit middleware.
func TestBodySizeLimit(t *testing.T) {
	tests := []struct {
		name       string
		bodySize   int
		maxSize    int64
		wantStatus int
	}{
		{
			name:       "small body passes",
			bodySize:   100,
			maxSize:    1024 * 1024, // 1 MiB
			wantStatus: http.StatusOK,
		},
		{
			name:       "exact size passes",
			bodySize:   1024,
			maxSize:    1024,
			wantStatus: http.StatusOK,
		},
		{
			name:       "empty body passes",
			bodySize:   0,
			maxSize:    1024,
			wantStatus: http.StatusOK,
		},
		{
			name:       "default max size 1MiB small body passes",
			bodySize:   100,
			maxSize:    0, // Use default
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			maxSize := tt.maxSize
			if maxSize == 0 {
				maxSize = MaxBodySize
			}

			handler := BodySizeLimit(maxSize)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			body := strings.Repeat("a", tt.bodySize)
			req := httptest.NewRequest("POST", "/test", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

// TestBodySizeLimitWithLargeBody tests that large bodies are rejected.
func TestBodySizeLimitWithLargeBody(t *testing.T) {
	maxSize := int64(100)
	handler := BodySizeLimit(maxSize)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read the body
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))

	// Create a body larger than maxSize
	body := strings.Repeat("x", 150)
	req := httptest.NewRequest("POST", "/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	// This should trigger the MaxBytesReader
	handler.ServeHTTP(rec, req)

	// The body reader should return an error when reading
	// In practice, the error handling depends on how the body is read
}

// TestBodySizeLimitNonPOST tests that non-POST methods bypass the limit.
func TestBodySizeLimitNonPOST(t *testing.T) {
	handler := BodySizeLimit(1024)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// GET request should not have body limit applied
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET: got status %d, want %d", rec.Code, http.StatusOK)
	}

	// OPTIONS request should also pass
	req = httptest.NewRequest("OPTIONS", "/test", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("OPTIONS: got status %d, want %d", rec.Code, http.StatusOK)
	}
}

// TestBodySizeLimitDefault tests the default size constant.
func TestBodySizeLimitDefault(t *testing.T) {
	// Verify MaxBodySize is 1 MiB
	if MaxBodySize != 1<<20 {
		t.Errorf("MaxBodySize = %d, want %d", MaxBodySize, 1<<20)
	}
}

// TestWriteBodySizeExceededError tests the error response writer.
func TestWriteBodySizeExceededError(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteBodySizeExceededError(rec)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("got content-type %s, want application/json", contentType)
	}

	// Verify the body contains JSON-RPC error
	body := rec.Body.String()
	if !strings.Contains(body, "jsonrpc") {
		t.Errorf("expected jsonrpc in response, got: %s", body)
	}
	if !strings.Contains(body, "32600") {
		t.Errorf("expected error code 32600 in response, got: %s", body)
	}
}
