package telemetry

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
)

// MetricsMiddleware wraps an http.Handler to record RPC metrics.
type MetricsMiddleware struct {
	handler http.Handler
}

// NewMetricsMiddleware creates a new metrics middleware.
func NewMetricsMiddleware(handler http.Handler) *MetricsMiddleware {
	return &MetricsMiddleware{handler: handler}
}

// ServeHTTP records metrics before and after handling the request.
func (m *MetricsMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Extract method from URL path or request
	method := extractMethod(r)

	// Start timer
	start := time.Now()

	// Create response wrapper to capture status code
	rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

	// Serve the request
	m.handler.ServeHTTP(rw, r)

	// Record metrics
	duration := time.Since(start).Seconds()

	// Increment counter
	RPCRequestsTotal.WithLabelValues(method).Inc()

	// Record histogram
	RPCDuration.WithLabelValues(method).Observe(duration)
}

// responseWriter wraps http.ResponseWriter to capture status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// extractMethod extracts the RPC method from the request.
func extractMethod(r *http.Request) string {
	// Try to get from request body for JSON-RPC
	if r.Method == http.MethodPost {
		// For JSON-RPC, the method is in the body, but we've already read it
		// For now, use the URL path
		path := r.URL.Path
		if path == "/" || path == "" {
			return "json-rpc"
		}
		return path
	}
	return r.Method
}

// RequestIDKey is the context key for request ID.
const RequestIDKey = "request_id"

// RequestIDMiddleware adds a request ID to each request.
type RequestIDMiddleware struct {
	handler http.Handler
}

// NewRequestIDMiddleware creates a new request ID middleware.
func NewRequestIDMiddleware(handler http.Handler) *RequestIDMiddleware {
	return &RequestIDMiddleware{handler: handler}
}

// ServeHTTP adds a request ID to the context and response headers.
func (m *RequestIDMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Generate UUID for request
	reqID := uuid.New().String()

	// Add to request context
	ctx := context.WithValue(r.Context(), RequestIDKey, reqID)

	// Add to response headers
	w.Header().Set("X-Request-ID", reqID)

	// Serve with modified context
	m.handler.ServeHTTP(w, r.WithContext(ctx))
}

// GetRequestID retrieves the request ID from the context.
func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(RequestIDKey).(string); ok {
		return id
	}
	return ""
}

// ExplorerMetricsMiddleware records explorer API metrics.
type ExplorerMetricsMiddleware struct {
	handler http.Handler
}

// NewExplorerMetricsMiddleware creates a new explorer metrics middleware.
func NewExplorerMetricsMiddleware(handler http.Handler) *ExplorerMetricsMiddleware {
	return &ExplorerMetricsMiddleware{handler: handler}
}

// ServeHTTP records metrics for explorer requests.
func (m *ExplorerMetricsMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Extract path pattern for labels
	path := r.URL.Path

	rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
	m.handler.ServeHTTP(rw, r)

	duration := time.Since(start).Seconds()

	ExplorerRequestsTotal.WithLabelValues(path, r.Method).Inc()
	ExplorerDuration.WithLabelValues(path, r.Method).Observe(duration)
}

// PrometheusMetricsHandler returns the Prometheus metrics handler.
func PrometheusMetricsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// This is handled by the promhttp middleware in the metrics server
		// Just redirect to the metrics endpoint
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.WriteHeader(http.StatusOK)
	})
}

// StatusCodeExtractor extracts the HTTP status code from response.
func StatusCodeExtractor(statusCode int) string {
	return strconv.Itoa(statusCode)
}