package middleware

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
)

// AuthMiddleware provides API key authentication for RPC endpoints.
type AuthMiddleware struct {
	apiKey  string
	header  string
	enabled bool
}

// NewAuthMiddleware creates a new auth middleware.
// If apiKey is empty, the middleware is disabled and passes all requests through.
func NewAuthMiddleware(apiKey, header string) *AuthMiddleware {
	if header == "" {
		header = "X-API-Key"
	}
	return &AuthMiddleware{
		apiKey:  apiKey,
		header:  header,
		enabled: apiKey != "",
	}
}

// Middleware returns the auth middleware handler.
func (am *AuthMiddleware) Middleware(next http.Handler) http.Handler {
	// If auth is disabled, pass through all requests
	if !am.enabled {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get the API key from the configured header
		givenKey := r.Header.Get(am.header)

		// Use constant-time compare to avoid timing attacks
		if len(givenKey) == 0 || subtle.ConstantTimeCompare([]byte(givenKey), []byte(am.apiKey)) != 1 {
			// Return 403 Forbidden with JSON-RPC error format
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(JSONRPCError{
				JSONRPC: "2.0",
				Error:   &RPCErr{Code: -32001, Message: "unauthorized"},
				ID:      nil,
			})
			return
		}

		next.ServeHTTP(w, r)
	})
}

// AuthMiddlewareFromConfig creates an AuthMiddleware from config values.
func AuthMiddlewareFromConfig(apiKey, header string) *AuthMiddleware {
	return NewAuthMiddleware(apiKey, header)
}
