package middleware

import (
	"encoding/json"
	"io"
	"net/http"
)

// MaxBodySize is the maximum allowed request body size (1 MiB)
const MaxBodySize = 1 << 20 // 1 MiB

// BodySizeLimit returns a middleware that limits request body size.
// It uses http.MaxBytesReader to prevent reading bodies larger than maxSize.
func BodySizeLimit(maxSize int64) func(http.Handler) http.Handler {
	if maxSize <= 0 {
		maxSize = MaxBodySize
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only apply to request methods that have a body
			if r.Method == "POST" || r.Method == "PUT" || r.Method == "PATCH" {
				// Use MaxBytesReader to limit the body size
				r.Body = http.MaxBytesReader(w, r.Body, maxSize)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// limitedReadCloser wraps an io.ReadCloser to track bytes read
type limitedReadCloser struct {
	reader io.ReadCloser
	limit  int64
	read   int64
}

func (l *limitedReadCloser) Read(p []byte) (n int, err error) {
	n, err = l.reader.Read(p)
	l.read += int64(n)
	// If we've exceeded the limit, the underlying MaxBytesReader will return an error
	return n, err
}

func (l *limitedReadCloser) Close() error {
	return l.reader.Close()
}

// WriteBodySizeExceededError writes a JSON-RPC error response for body size exceeded.
func WriteBodySizeExceededError(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusRequestEntityTooLarge)
	json.NewEncoder(w).Encode(JSONRPCError{
		JSONRPC: "2.0",
		Error: &RPCErr{
			Code:    InvalidRequest,
			Message: "request body exceeds maximum size (1 MiB)",
		},
		ID: nil,
	})
}
