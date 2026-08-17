package rpc

import (
	"net/http"

	"github.com/BryanOx/dsn/rpc/middleware"
	"github.com/BryanOx/dsn/rpc/service"
	"github.com/BryanOx/dsn/rpc/ws"
	"github.com/BryanOx/dsn/telemetry"
	"github.com/gorilla/mux"
)

// Router holds the router and associated components.
type Router struct {
	router       *mux.Router
	hub          *ws.Hub
	subscription *ws.SubscriptionHandler
	rateLimiter  *middleware.RateLimiter
}

// RouterConfig holds optional configuration for the router.
type RouterConfig struct {
	TLSEnabled   bool
	ApiKey       string
	ApiKeyHeader string
	CORSOrigins  []string
}

// DefaultRouterConfig returns a default router config.
func DefaultRouterConfig() RouterConfig {
	return RouterConfig{
		TLSEnabled:   false,
		ApiKey:       "",
		ApiKeyHeader: "X-API-Key",
	}
}

// NewRouter creates a gorilla/mux router with all RPC and REST routes.
// The service parameter is used to handle RPC method calls.
// The optional config parameter enables TLS and API key authentication.
func NewRouter(svc service.NodeService, cfg ...RouterConfig) *Router {
	router := mux.NewRouter()

	// Get config or use defaults
	config := DefaultRouterConfig()
	if len(cfg) > 0 {
		config = cfg[0]
	}

	// Create WebSocket hub with origin restrictions
	hub := ws.NewHub(config.CORSOrigins)
	go hub.Run()

	// Create subscription handler
	subscription := ws.NewSubscriptionHandler(hub)

	// Create rate limiter
	rateLimiter := middleware.NewRateLimiter(middleware.DefaultRateLimiterConfig())

	r := &Router{
		router:       router,
		hub:          hub,
		subscription: subscription,
		rateLimiter:  rateLimiter,
	}

	// Build middleware chain (outermost to innermost):
	// 1. CORS (applied first — handles OPTIONS preflight without auth)
	// 2. Security headers
	// 3. Rate limiting (100 req/s, burst 200)
	// 4. API key authentication
	// 5. Request body size limit (1 MiB)
	// 6. Request ID generation
	// 7. Metrics collection
	// 8. JSON-RPC handling

	// Start with the JSON-RPC handler as http.Handler
	var rpcHandler http.Handler = handleJSONRPC(svc)

	// Add metrics middleware
	rpcHandler = telemetry.NewMetricsMiddleware(rpcHandler)

	// Add request ID middleware
	rpcHandler = telemetry.NewRequestIDMiddleware(rpcHandler)

	// Add body size limit
	rpcHandler = middleware.BodySizeLimit(middleware.MaxBodySize)(rpcHandler)

	// Add API key authentication (if configured)
	if config.ApiKey != "" {
		authMiddleware := middleware.NewAuthMiddleware(config.ApiKey, config.ApiKeyHeader)
		rpcHandler = authMiddleware.Middleware(rpcHandler)
	}

	// Add rate limiting
	rpcHandler = rateLimiter.Middleware(rpcHandler)

	// Add security headers
	rpcHandler = middleware.SecurityHeadersMiddleware(config.TLSEnabled)(rpcHandler)

	// Add CORS middleware (outermost — handles OPTIONS preflight before auth)
	if len(config.CORSOrigins) > 0 {
		corsConfig := middleware.CORSConfig{
			AllowedOrigins: config.CORSOrigins,
			AllowedMethods: []string{"POST", "OPTIONS"},
			AllowedHeaders: []string{"Content-Type"},
			MaxAge:         86400,
		}
		rpcHandler = middleware.CORS(corsConfig)(rpcHandler)
	}

	router.Handle("/", rpcHandler).Methods(http.MethodPost)

	// WebSocket endpoint with security headers
	var wsHandler http.Handler = http.HandlerFunc(hub.HandleWebSocket)
	wsHandler = middleware.SecurityHeadersMiddleware(config.TLSEnabled)(wsHandler)
	router.Handle("/ws", wsHandler).Methods(http.MethodGet, http.MethodPost)

	// Health check endpoint with security headers
	var healthHandler http.Handler = http.HandlerFunc(handleHealth)
	healthHandler = middleware.SecurityHeadersMiddleware(config.TLSEnabled)(healthHandler)
	router.Handle("/health", healthHandler).Methods(http.MethodGet)

	return r
}

// ServeHTTP dispatches the request to the handler whose
// pattern most closely matches the request URL.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.router.ServeHTTP(w, req)
}

// SubscriptionHandler returns the WebSocket subscription handler
// for broadcasting events from the indexer/node.
func (r *Router) SubscriptionHandler() *ws.SubscriptionHandler {
	return r.subscription
}

// Hub returns the WebSocket hub.
func (r *Router) Hub() *ws.Hub {
	return r.hub
}

// Stop stops the router's background goroutines.
func (r *Router) Stop() {
	if r.rateLimiter != nil {
		r.rateLimiter.Stop()
	}
}

// handleJSONRPC handles JSON-RPC POST requests.
func handleJSONRPC(svc service.NodeService) http.HandlerFunc {
	server := &Server{
		service: svc,
		handler: NewHandler(svc),
	}
	return func(w http.ResponseWriter, r *http.Request) {
		server.ServeHTTP(w, r)
	}
}

// handleHealth returns a simple health check response.
func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}
