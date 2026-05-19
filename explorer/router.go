package explorer

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/dsn/dsn/indexer"
	"github.com/dsn/dsn/rpc/middleware"
	"github.com/dsn/dsn/rpc/service"
	"github.com/dsn/dsn/telemetry"
	"github.com/gorilla/mux"
)

// NewRouter creates a new gorilla/mux router with all explorer routes.
func NewRouter(idx *indexer.Indexer, svc service.NodeService) *mux.Router {
	r := mux.NewRouter()

	// API v1 routes
	api := r.PathPrefix("/api/v1").Subrouter()

	// Blocks endpoints
	api.HandleFunc("/blocks", handleBlocks(idx, svc)).Methods("GET")
	api.HandleFunc("/blocks/{numberOrHash}", handleBlockDetail(idx, svc)).Methods("GET")

	// Transaction endpoints
	api.HandleFunc("/transactions/{hash}", handleTransaction(idx, svc)).Methods("GET")

	// Account endpoints
	api.HandleFunc("/accounts/{address}", handleAccount(idx, svc)).Methods("GET")

	// Contract endpoints
	api.HandleFunc("/contracts/{address}", handleContract(idx, svc)).Methods("GET")

	// Validator endpoints
	api.HandleFunc("/validators", handleValidators(svc)).Methods("GET")

	// Events endpoints
	api.HandleFunc("/events", handleEvents(idx, svc)).Methods("GET")

	// Supply endpoints
	api.HandleFunc("/supply", handleSupply(svc)).Methods("GET")

	// Middleware (order matters - applied in reverse)
	r.Use(metricsMiddleware)             // Record metrics
	r.Use(requestIDMiddleware)          // Add request ID
	r.Use(corsMiddleware)               // Handle CORS
	r.Use(contentTypeMiddleware)        // Set content type
	r.Use(middleware.BodySizeLimit(middleware.MaxBodySize)) // Limit body size (1 MiB)

	// Health check
	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	return r
}

// contentTypeMiddleware sets JSON content type for all API responses.
func contentTypeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

// corsMiddleware handles CORS headers.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			// In production, this would check against a whitelist
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		}
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// loggingMiddleware logs incoming requests.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		log.Printf("[EXPLORER] %s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
		log.Printf("[EXPLORER] %s %s completed in %v", r.Method, r.URL.Path, time.Since(start))
	})
}

// requestIDMiddleware adds a request ID to each request.
func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := r.Header.Get("X-Request-ID")
		if reqID == "" {
			reqID = generateRequestID()
		}
		w.Header().Set("X-Request-ID", reqID)
		next.ServeHTTP(w, r)
	})
}

// metricsMiddleware records explorer metrics.
func metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &statusResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(w, r)
		duration := time.Since(start).Seconds()
		telemetry.ExplorerRequestsTotal.WithLabelValues(r.URL.Path, r.Method).Inc()
		telemetry.ExplorerDuration.WithLabelValues(r.URL.Path, r.Method).Observe(duration)
		_ = rw // unused for now
	})
}

// statusResponseWriter wraps http.ResponseWriter to capture status code.
type statusResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *statusResponseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// generateRequestID generates a unique request ID.
func generateRequestID() string {
	return "req-" + time.Now().Format("20060102150405") + "-" + randomString(8)
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[time.Now().UnixNano()%int64(len(letters))]
		time.Sleep(time.Nanosecond) // ensure different values
	}
	return string(b)
}

// WriteJSON writes a JSON response with proper error handling.
func WriteJSON(w http.ResponseWriter, status int, data interface{}) {
	w.WriteHeader(status)
	if data != nil {
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)
		enc.Encode(data)
	}
}

// WriteError writes a JSON error response.
func WriteError(w http.ResponseWriter, status int, errMsg string, code int) {
	WriteJSON(w, status, ErrorResponse{Error: errMsg, Code: code})
}