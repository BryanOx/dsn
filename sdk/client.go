package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ClientOption is a functional option for configuring Client.
type ClientOption func(*Client)

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(c *Client) {
		c.httpClient = httpClient
	}
}

// WithTimeout sets the request timeout.
func WithTimeout(timeout time.Duration) ClientOption {
	return func(c *Client) {
		c.httpClient.Timeout = timeout
	}
}

// WithRetry sets retry options.
func WithRetry(maxAttempts int, backoff time.Duration) ClientOption {
	return func(c *Client) {
		c.retryEnabled = true
		c.maxRetries = maxAttempts
		c.retryBackoff = backoff
	}
}

// Client wraps an HTTP client for making JSON-RPC calls.
type Client struct {
	baseURL      string
	wsURL        string
	httpClient   *http.Client
	retryEnabled bool
	maxRetries   int
	retryBackoff time.Duration
}

// New creates a new Client with default settings.
func New(baseURL string) *Client {
	return NewWithClient(baseURL, &http.Client{
		Timeout: 10 * time.Second,
	})
}

// NewWithClient creates a new Client with a custom HTTP client.
func NewWithClient(baseURL string, httpClient *http.Client) *Client {
	wsURL := deriveWSURL(baseURL)
	return &Client{
		baseURL:    baseURL,
		wsURL:      wsURL,
		httpClient: httpClient,
	}
}

// BaseURL returns the HTTP endpoint.
func (c *Client) BaseURL() string {
	return c.baseURL
}

// WSURL returns the WebSocket endpoint.
func (c *Client) WSURL() string {
	return c.wsURL
}

// deriveWSURL converts http/https to ws/wss.
func deriveWSURL(httpURL string) string {
	if strings.HasPrefix(httpURL, "https://") {
		return "wss://" + strings.TrimPrefix(httpURL, "https://")
	}
	if strings.HasPrefix(httpURL, "http://") {
		return "ws://" + strings.TrimPrefix(httpURL, "http://")
	}
	return httpURL
}

// call performs a JSON-RPC call and unmarshals the result.
func (c *Client) call(ctx context.Context, method string, params, result interface{}) error {
	// Build request
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      1,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	// Execute with optional retry
	var respBody []byte

	if c.retryEnabled {
		respBody, err = c.doWithRetry(ctx, body)
	} else {
		respBody, err = c.doRequest(ctx, body)
	}
	if err != nil {
		return err
	}

	// Parse response
	var rpcResp JSONRPCResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}

	if rpcResp.Error != nil {
		return parseRPCError(rpcResp.Error)
	}

	if result == nil {
		return nil
	}

	// Unmarshal result
	if err := json.Unmarshal(rpcResp.Result, result); err != nil {
		return fmt.Errorf("unmarshal result: %w", err)
	}

	return nil
}

// doRequest performs a single HTTP request.
func (c *Client) doRequest(ctx context.Context, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/rpc", strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("server error: %d", resp.StatusCode)
	}

	// Read response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	return bodyBytes, nil
}

// doWithRetry performs request with exponential backoff retry.
func (c *Client) doWithRetry(ctx context.Context, body []byte) ([]byte, error) {
	var lastErr error
	var respBody []byte

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			sleep := c.retryBackoff * time.Duration(1<<uint(attempt-1))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(sleep):
			}
		}

		respBody, lastErr = c.doRequest(ctx, body)
		if lastErr == nil {
			return respBody, nil
		}

		// Check if error is retryable
		if !isRetryableError(lastErr) {
			return nil, lastErr
		}
	}

	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// HTTPClient returns the underlying HTTP client.
func (c *Client) HTTPClient() *http.Client {
	return c.httpClient
}
