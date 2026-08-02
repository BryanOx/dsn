package middleware

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RateLimiterConfig holds rate limiter configuration.
type RateLimiterConfig struct {
	RequestsPerSecond int           // Max requests per second
	BurstSize         int           // Max burst size
	CleanupInterval   time.Duration // How often to clean up old entries
}

// DefaultRateLimiterConfig returns default configuration.
func DefaultRateLimiterConfig() RateLimiterConfig {
	return RateLimiterConfig{
		RequestsPerSecond: 100,
		BurstSize:         200,
		CleanupInterval:   5 * time.Minute,
	}
}

// RateLimiter implements a token bucket rate limiter per IP.
type RateLimiter struct {
	config      RateLimiterConfig
	buckets     map[string]*tokenBucket
	mu          sync.RWMutex
	stopCleanup chan struct{}
}

// tokenBucket implements token bucket algorithm.
type tokenBucket struct {
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per second
	lastRefill time.Time
	mu         sync.Mutex
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter(config RateLimiterConfig) *RateLimiter {
	if config.RequestsPerSecond <= 0 {
		config.RequestsPerSecond = 100
	}
	if config.BurstSize <= 0 {
		config.BurstSize = 200
	}
	if config.CleanupInterval <= 0 {
		config.CleanupInterval = 5 * time.Minute
	}

	rl := &RateLimiter{
		config:      config,
		buckets:     make(map[string]*tokenBucket),
		stopCleanup: make(chan struct{}),
	}

	// Start cleanup goroutine
	go rl.cleanup()

	return rl
}

// Stop stops the rate limiter cleanup goroutine.
func (rl *RateLimiter) Stop() {
	close(rl.stopCleanup)
}

// Middleware returns the rate limiting middleware.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := getClientIP(r)

		if !rl.allow(ip) {
			retryAfter := rl.retryAfterSeconds(ip)
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"jsonrpc":"2.0","error":{"code":-32000,"message":"rate limit exceeded"},"id":null}`))
			return
		}

		next.ServeHTTP(w, r)
	})
}

// allow checks if the IP is allowed to make a request.
func (rl *RateLimiter) allow(ip string) bool {
	rl.mu.RLock()
	bucket, exists := rl.buckets[ip]
	rl.mu.RUnlock()

	if !exists {
		rl.mu.Lock()
		// Double-check after acquiring write lock
		if bucket, exists = rl.buckets[ip]; !exists {
			bucket = newTokenBucket(rl.config.BurstSize, rl.config.RequestsPerSecond)
			rl.buckets[ip] = bucket
		}
		rl.mu.Unlock()
		// Consume one token from the new bucket
		return bucket.consume()
	}

	return bucket.consume()
}

// retryAfterSeconds returns the number of seconds until the client can retry.
func (rl *RateLimiter) retryAfterSeconds(ip string) int {
	rl.mu.RLock()
	bucket, exists := rl.buckets[ip]
	rl.mu.RUnlock()

	if !exists {
		return 1
	}

	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	// Calculate seconds until tokens are available
	tokensNeeded := 1.0 - bucket.tokens
	if tokensNeeded <= 0 {
		return 0
	}
	seconds := int(tokensNeeded / bucket.refillRate)
	if seconds < 1 {
		seconds = 1
	}
	return seconds
}

// cleanup removes old entries periodically.
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-rl.stopCleanup:
			return
		case <-ticker.C:
			rl.mu.Lock()
			now := time.Now()
			for ip, bucket := range rl.buckets {
				bucket.mu.Lock()
				// Remove if no activity for 10 minutes
				if now.Sub(bucket.lastRefill) > 10*time.Minute {
					delete(rl.buckets, ip)
				}
				bucket.mu.Unlock()
			}
			rl.mu.Unlock()
		}
	}
}

// newTokenBucket creates a new token bucket with the given capacity and refill rate.
func newTokenBucket(capacity int, refillRate int) *tokenBucket {
	return &tokenBucket{
		tokens:     float64(capacity),
		maxTokens:  float64(capacity),
		refillRate: float64(refillRate),
		lastRefill: time.Now(),
	}
}

// consume attempts to consume one token.
func (tb *tokenBucket) consume() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	// Refill tokens based on time elapsed
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.tokens = tb.tokens + elapsed*tb.refillRate
	if tb.tokens > tb.maxTokens {
		tb.tokens = tb.maxTokens
	}
	tb.lastRefill = now

	// Try to consume a token
	if tb.tokens >= 1 {
		tb.tokens--
		return true
	}

	return false
}

// RateLimiterMiddleware creates a rate limiting middleware with default config.
func RateLimiterMiddleware() func(http.Handler) http.Handler {
	rl := NewRateLimiter(DefaultRateLimiterConfig())
	return rl.Middleware
}

// getClientIP extracts the client IP from request.
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first (for proxies)
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		// Take first IP in chain
		ips := strings.Split(xff, ",")
		for _, ip := range ips {
			ip = strings.TrimSpace(ip)
			if ip != "" {
				return ip
			}
		}
	}

	// Check X-Real-IP header
	xri := r.Header.Get("X-Real-IP")
	if xri != "" {
		return strings.TrimSpace(xri)
	}

	// Fall back to RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
