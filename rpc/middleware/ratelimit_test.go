package middleware

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// TestRateLimiterBasic tests basic rate limiting functionality.
func TestRateLimiterBasic(t *testing.T) {
	config := RateLimiterConfig{
		RequestsPerSecond: 10,
		BurstSize:         20,
		CleanupInterval:   1 * time.Minute,
	}

	rl := NewRateLimiter(config)
	defer rl.Stop()

	// Create a test handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Apply rate limiter middleware
	middleware := rl.Middleware(handler)

	// Test first request - should be allowed
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	middleware.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

// TestRateLimiterExceedLimit tests that rate limiting kicks in after burst.
func TestRateLimiterExceedLimit(t *testing.T) {
	config := RateLimiterConfig{
		RequestsPerSecond: 5,
		BurstSize:         5, // Small burst for testing
		CleanupInterval:   1 * time.Minute,
	}

	rl := NewRateLimiter(config)
	defer rl.Stop()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := rl.Middleware(handler)

	// Exhaust the burst
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()
		middleware.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("request %d: expected status 200, got %d", i+1, rec.Code)
		}
	}

	// The next request should be rate limited
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	middleware.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("expected status 429, got %d", rec.Code)
	}

	// Check Retry-After header
	retryAfter := rec.Header().Get("Retry-After")
	if retryAfter == "" {
		t.Error("expected Retry-After header")
	}
}

// TestRateLimiterDifferentIPs tests that different IPs have separate limits.
func TestRateLimiterDifferentIPs(t *testing.T) {
	config := RateLimiterConfig{
		RequestsPerSecond: 2,
		BurstSize:         2,
		CleanupInterval:   1 * time.Minute,
	}

	rl := NewRateLimiter(config)
	defer rl.Stop()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := rl.Middleware(handler)

	// Exhaust rate limit for IP 1
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.1:1234"
		rec := httptest.NewRecorder()
		middleware.ServeHTTP(rec, req)
	}

	// IP 1 should be rate limited now
	req1 := httptest.NewRequest("GET", "/test", nil)
	req1.RemoteAddr = "192.168.1.1:1234"
	rec1 := httptest.NewRecorder()
	middleware.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusTooManyRequests {
		t.Errorf("IP 1: expected status 429, got %d", rec1.Code)
	}

	// IP 2 should still be allowed
	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.RemoteAddr = "192.168.1.2:1234"
	rec2 := httptest.NewRecorder()
	middleware.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Errorf("IP 2: expected status 200, got %d", rec2.Code)
	}
}

// TestRateLimiterXForwardedFor tests X-Forwarded-For header usage.
func TestRateLimiterXForwardedFor(t *testing.T) {
	config := RateLimiterConfig{
		RequestsPerSecond: 2,
		BurstSize:         2,
		CleanupInterval:   1 * time.Minute,
	}

	rl := NewRateLimiter(config)
	defer rl.Stop()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := rl.Middleware(handler)

	// First request with X-Forwarded-For
	req1 := httptest.NewRequest("GET", "/test", nil)
	req1.Header.Set("X-Forwarded-For", "10.0.0.1")
	rec1 := httptest.NewRecorder()
	middleware.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Errorf("first request: expected status 200, got %d", rec1.Code)
	}

	// Second request from same IP (via X-Forwarded-For) should be limited
	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.Header.Set("X-Forwarded-For", "10.0.0.1")
	rec2 := httptest.NewRecorder()
	middleware.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Errorf("second request: expected status 200 (within burst), got %d", rec2.Code)
	}

	// Third request should be limited
	req3 := httptest.NewRequest("GET", "/test", nil)
	req3.Header.Set("X-Forwarded-For", "10.0.0.1")
	rec3 := httptest.NewRecorder()
	middleware.ServeHTTP(rec3, req3)

	if rec3.Code != http.StatusTooManyRequests {
		t.Errorf("third request: expected status 429, got %d", rec3.Code)
	}
}

// TestRateLimiterConcurrent tests concurrent access to rate limiter.
func TestRateLimiterConcurrent(t *testing.T) {
	config := RateLimiterConfig{
		RequestsPerSecond: 100,
		BurstSize:         100,
		CleanupInterval:   1 * time.Minute,
	}

	rl := NewRateLimiter(config)
	defer rl.Stop()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := rl.Middleware(handler)

	// Make concurrent requests
	var wg sync.WaitGroup
	successCount := 0
	var mu sync.Mutex

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("GET", "/test", nil)
			rec := httptest.NewRecorder()
			middleware.ServeHTTP(rec, req)

			mu.Lock()
			if rec.Code == http.StatusOK {
				successCount++
			}
			mu.Unlock()
		}()
	}

	wg.Wait()

	// Most requests should succeed (within burst limit)
	if successCount < 40 {
		t.Errorf("expected at least 40 successes, got %d", successCount)
	}
}

// TestRateLimiterTokenBucketRefill tests that tokens are refilled over time.
func TestRateLimiterTokenBucketRefill(t *testing.T) {
	config := RateLimiterConfig{
		RequestsPerSecond: 10, // 10 tokens per second
		BurstSize:         5,  // Start with 5
		CleanupInterval:   1 * time.Minute,
	}

	rl := NewRateLimiter(config)
	defer rl.Stop()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := rl.Middleware(handler)

	// Exhaust the burst (5 requests)
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()
		middleware.ServeHTTP(rec, req)
	}

	// Wait for tokens to refill (need 1 token = 0.1 second)
	time.Sleep(200 * time.Millisecond)

	// Should be able to make at least one request now
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	middleware.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 after refill, got %d", rec.Code)
	}
}
