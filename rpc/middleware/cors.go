package middleware

import (
	"net/http"
	"strings"
	"sync"
)

// CORSConfig holds CORS configuration.
type CORSConfig struct {
	AllowedOrigins []string
	AllowedMethods []string
	AllowedHeaders []string
	ExposedHeaders []string
	MaxAge         int
}

// DefaultCORSConfig returns a default CORS configuration.
func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders: []string{"Content-Type", "Authorization"},
		ExposedHeaders: []string{},
		MaxAge:         86400,
	}
}

// CORS returns a CORS middleware with the given configuration.
func CORS(config CORSConfig) func(http.Handler) http.Handler {
	// Build a quick-lookup map for allowed origins
	allowedOrigins := make(map[string]bool)
	for _, origin := range config.AllowedOrigins {
		allowedOrigins[strings.ToLower(origin)] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Check if origin is allowed
			allowed := false
			if origin != "" {
				lowerOrigin := strings.ToLower(origin)
				if allowedOrigins["*"] {
					allowed = true
				} else if allowedOrigins[lowerOrigin] {
					allowed = true
				}
			}

			// Handle preflight
			if r.Method == http.MethodOptions {
				if !allowed {
					// Origin not allowed - still process but don't add CORS headers
					w.WriteHeader(http.StatusNoContent)
					return
				}

				// Set preflight headers
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", strings.Join(config.AllowedMethods, ", "))
				w.Header().Set("Access-Control-Allow-Headers", strings.Join(config.AllowedHeaders, ", "))
				if len(config.ExposedHeaders) > 0 {
					w.Header().Set("Access-Control-Expose-Headers", strings.Join(config.ExposedHeaders, ", "))
				}
				if config.MaxAge > 0 {
					w.Header().Set("Access-Control-Max-Age", string(rune(config.MaxAge)))
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}

			// Add CORS headers to response
			if allowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				if len(config.ExposedHeaders) > 0 {
					w.Header().Set("Access-Control-Expose-Headers", strings.Join(config.ExposedHeaders, ", "))
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// CORSWithLock provides thread-safe CORS configuration updates.
type CORSWithLock struct {
	config CORSConfig
	mu     sync.RWMutex
}

// NewCORSWithLock creates a new CORS middleware with lock for dynamic updates.
func NewCORSWithLock(config CORSConfig) *CORSWithLock {
	return &CORSWithLock{config: config}
}

// Middleware returns the CORS middleware.
func (c *CORSWithLock) Middleware(next http.Handler) http.Handler {
	config := c.GetConfig()
	return CORS(config)(next)
}

// GetConfig returns a copy of the current configuration.
func (c *CORSWithLock) GetConfig() CORSConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.config
}

// SetAllowedOrigins updates the allowed origins.
func (c *CORSWithLock) SetAllowedOrigins(origins []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.config.AllowedOrigins = origins
}