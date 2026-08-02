package sdk

import (
	"context"
	"time"
)

// RetryConfig holds retry configuration.
type RetryConfig struct {
	MaxAttempts int
	Backoff     time.Duration
	MaxBackoff  time.Duration
}

// DefaultRetryConfig returns default retry settings.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts: 3,
		Backoff:     time.Second,
		MaxBackoff:  10 * time.Second,
	}
}

// ExponentialBackoff calculates the next backoff duration.
func (c *RetryConfig) ExponentialBackoff(attempt int) time.Duration {
	backoff := c.Backoff * time.Duration(1<<uint(attempt-1))
	if backoff > c.MaxBackoff {
		return c.MaxBackoff
	}
	return backoff
}

// RetryHandler wraps a function with retry logic.
type RetryHandler struct {
	config RetryConfig
}

// NewRetryHandler creates a new retry handler.
func NewRetryHandler(config RetryConfig) *RetryHandler {
	if config.MaxAttempts == 0 {
		config.MaxAttempts = 3
	}
	if config.Backoff == 0 {
		config.Backoff = time.Second
	}
	return &RetryHandler{config: config}
}

// Do executes the function with retry logic.
func (h *RetryHandler) Do(ctx context.Context, fn func() error) error {
	var lastErr error

	for attempt := 1; attempt <= h.config.MaxAttempts; attempt++ {
		if err := fn(); err != nil {
			lastErr = err

			// Check if error is retryable
			if !isRetryableError(err) {
				return err
			}

			// Wait before next attempt
			if attempt < h.config.MaxAttempts {
				backoff := h.config.ExponentialBackoff(attempt)
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(backoff):
				}
			}
			continue
		}

		// Success
		return nil
	}

	return lastErr
}

// isRetryableError checks if an error should trigger a retry.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// Retry on connection errors, timeouts, and server errors
	retryableIndicators := []string{
		"connection refused",
		"connection reset",
		"connection timeout",
		"i/o timeout",
		"no such host",
		"network is unreachable",
		"server error",
		"temporary failure",
		"503",
		"502",
		"504",
	}

	for _, indicator := range retryableIndicators {
		if containsIgnoreCase(errStr, indicator) {
			return true
		}
	}

	return false
}

// containsIgnoreCase checks if a string contains a substring (case-insensitive).
func containsIgnoreCase(s, substr string) bool {
	// Simple case-insensitive check
	sLower := toLower(s)
	substrLower := toLower(substr)
	return contains(sLower, substrLower)
}

func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		result[i] = c
	}
	return string(result)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (len(substr) == 0 || findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// WithRetry creates a ClientOption that enables retry with custom config.
func WithRetryConfig(config RetryConfig) ClientOption {
	return func(c *Client) {
		c.retryEnabled = true
		c.maxRetries = config.MaxAttempts
		c.retryBackoff = config.Backoff
	}
}
