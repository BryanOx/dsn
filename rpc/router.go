package rpc

import (
	"net/http"

	"github.com/dsn/dsn/rpc/middleware"
	"github.com/dsn/dsn/rpc/service"
	"github.com/dsn/dsn/rpc/ws"
	"github.com/dsn/dsn/telemetry"
	"github.com/gorilla/mux"
)

// Router holds the router and associated components.
type Router struct {
	router       *mux.Router
	hub          *ws.Hub
	subscription *ws.SubscriptionHandler
	rateLimiter  *middleware.RateLimiter
}

// NewRouter creates a gorilla/mux router with all RPC and REST routes.
// The service parameter is used to handle RPC method calls.
func NewRouter(svc service.NodeService) *Router {
	router := mux.NewRouter()

	// Create WebSocket hub
	hub := ws.NewHub()
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

	// JSON-RPC handler with middleware chain:
	// 1. Rate limiting (100 req/s, burst 200)
	// 2. Request body size limit (1 MiB)
	// 3. Request ID generation
	// 4. Metrics collection
	// 5. JSON-RPC handling
	rpcHandler := middleware.BodySizeLimit(middleware.MaxBodySize)(
		rateLimiter.Middleware(
			telemetry.NewRequestIDMiddleware(
				telemetry.NewMetricsMiddleware(handleJSONRPC(svc)),
			),
		),
	)
	router.HandleFunc("/", rpcHandler.ServeHTTP).Methods(http.MethodPost)

	// WebSocket endpoint with request ID middleware
	router.HandleFunc("/ws", hub.HandleWebSocket)

	// Health check endpoint
	router.HandleFunc("/health", handleHealth).Methods(http.MethodGet)

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