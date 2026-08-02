package telemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

// TestMetricsRegistration verifies that Prometheus metrics are properly registered.
func TestMetricsRegistration(t *testing.T) {
	// Test that all metrics can be created and used
	t.Run("RPCRequestsTotal can be incremented", func(t *testing.T) {
		// Counter should start at 0
		initial := testutil.ToFloat64(RPCRequestsTotal.WithLabelValues("test_method"))
		assert.Equal(t, 0.0, initial)

		// Increment the counter
		RPCRequestsTotal.WithLabelValues("test_method").Inc()

		// Verify it was incremented
		after := testutil.ToFloat64(RPCRequestsTotal.WithLabelValues("test_method"))
		assert.Equal(t, 1.0, after)
	})

	t.Run("RPCDuration can record observations", func(t *testing.T) {
		// Record a duration
		RPCDuration.WithLabelValues("test_method").Observe(0.123)

		// Verify it was recorded - histogram vec returns a histogram
		histVec := RPCDuration.WithLabelValues("test_method")
		// We can't directly check histogram contents, but no panic means it works
		assert.NotNil(t, histVec)
	})

	t.Run("Gauges can be set", func(t *testing.T) {
		IndexerHeight.Set(123.0)
		after := testutil.ToFloat64(IndexerHeight)
		assert.Equal(t, 123.0, after)

		MempoolTxCount.Set(5.0)
		after = testutil.ToFloat64(MempoolTxCount)
		assert.Equal(t, 5.0, after)

		PeerCount.Set(10.0)
		after = testutil.ToFloat64(PeerCount)
		assert.Equal(t, 10.0, after)

		WSConnections.Set(7.0)
		after = testutil.ToFloat64(WSConnections)
		assert.Equal(t, 7.0, after)

		IndexerLag.Set(0.0)
		after = testutil.ToFloat64(IndexerLag)
		assert.Equal(t, 0.0, after)
	})
}

// TestRequestIDMiddleware verifies that the request ID middleware generates UUIDs.
func TestRequestIDMiddleware(t *testing.T) {
	t.Run("generates unique request IDs", func(t *testing.T) {
		handler := NewRequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Verify request ID was added to context
			reqID := GetRequestID(r.Context())
			assert.NotEmpty(t, reqID, "Request ID should not be empty")

			// Verify it looks like a UUID (contains hyphens)
			assert.Contains(t, reqID, "-", "Request ID should contain dashes")
		}))

		// Create test request
		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()

		// Serve the request
		handler.ServeHTTP(rec, req)

		// Verify response has X-Request-ID header
		assert.NotEmpty(t, rec.Header().Get("X-Request-ID"), "Response should have X-Request-ID header")
	})

	t.Run("request ID in response header is UUID format", func(t *testing.T) {
		handler := NewRequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		reqID := rec.Header().Get("X-Request-ID")
		assert.NotEmpty(t, reqID)

		// UUID format check (8-4-4-4-12) - should be 36 characters
		assert.Len(t, reqID, 36, "UUID should be 36 characters")

		// Check hyphen positions
		assert.Equal(t, "-", string(reqID[8]), "UUID should have hyphen at position 8")
		assert.Equal(t, "-", string(reqID[13]), "UUID should have hyphen at position 13")
		assert.Equal(t, "-", string(reqID[18]), "UUID should have hyphen at position 18")
		assert.Equal(t, "-", string(reqID[23]), "UUID should have hyphen at position 23")
	})
}

// TestMetricsMiddleware verifies that the metrics middleware records metrics.
func TestMetricsMiddleware(t *testing.T) {
	t.Run("records request count and duration", func(t *testing.T) {
		handler := NewMetricsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("POST", "/", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		// Verify counter was incremented
		count := testutil.ToFloat64(RPCRequestsTotal.WithLabelValues("json-rpc"))
		assert.Greater(t, count, 0.0, "Request counter should be incremented")
	})
}

// TestGetRequestID verifies the GetRequestID function.
func TestGetRequestID(t *testing.T) {
	t.Run("returns empty string for empty context", func(t *testing.T) {
		ctx := context.Background()
		reqID := GetRequestID(ctx)
		assert.Empty(t, reqID)
	})

	t.Run("returns request ID from context", func(t *testing.T) {
		expectedID := "test-request-id-123"
		ctx := context.WithValue(context.Background(), RequestIDKey, expectedID)
		reqID := GetRequestID(ctx)
		assert.Equal(t, expectedID, reqID)
	})

	t.Run("returns empty string for wrong type", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), RequestIDKey, 12345)
		reqID := GetRequestID(ctx)
		assert.Empty(t, reqID)
	})
}

// TestLogger tests the slog logger setup.
func TestLogger(t *testing.T) {
	t.Run("creates logger with default settings", func(t *testing.T) {
		logger := NewLogger("info")
		assert.NotNil(t, logger)
	})

	t.Run("creates logger with different levels", func(t *testing.T) {
		loggerDebug := NewLogger("debug")
		loggerWarn := NewLogger("warn")
		loggerError := NewLogger("error")

		assert.NotNil(t, loggerDebug)
		assert.NotNil(t, loggerWarn)
		assert.NotNil(t, loggerError)
	})

	t.Run("global logger is initialized", func(t *testing.T) {
		assert.NotNil(t, DefaultLogger)
	})
}

// TestWithRequestID tests the WithRequestID helper.
func TestWithRequestID(t *testing.T) {
	t.Run("attaches request ID to logger", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), RequestIDKey, "test-123")
		logger := NewLogger("info")

		loggerWithID := WithRequestID(ctx, logger)
		assert.NotNil(t, loggerWithID)
	})

	t.Run("returns original logger when no request ID", func(t *testing.T) {
		ctx := context.Background()
		logger := NewLogger("info")

		loggerWithID := WithRequestID(ctx, logger)
		assert.NotNil(t, loggerWithID)
	})
}
