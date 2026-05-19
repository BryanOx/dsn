package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/dsn/dsn/types"
)

// RPCRequest is a JSON-RPC 2.0 request.
type RPCRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
	ID      int           `json:"id"`
}

// RPCResponse is a JSON-RPC 2.0 response.
type RPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
	ID      int             `json:"id"`
}

// RPCError represents a JSON-RPC error.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("RPC error %d: %s", e.Code, e.Message)
}

// Client connects to a DSN node via JSON-RPC 2.0.
type Client struct {
	url    string
	httpDo func(*http.Request) (*http.Response, error)
}

// New creates a new RPC client pointing at the given URL.
func New(url string) *Client {
	return &Client{
		url: url,
		httpDo: (&http.Client{Timeout: 10 * time.Second}).Do,
	}
}

func (c *Client) call(method string, params []interface{}, result interface{}) error {
	req := RPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      1,
	}
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", c.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpDo(httpReq)
	if err != nil {
		return fmt.Errorf("http call: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	var rpcResp RPCResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}

	if rpcResp.Error != nil {
		return rpcResp.Error
	}

	if result != nil && len(rpcResp.Result) > 0 {
		if err := json.Unmarshal(rpcResp.Result, result); err != nil {
			return fmt.Errorf("unmarshal result: %w", err)
		}
	}

	return nil
}

// GetBalance returns the DSN balance for an address (as a string number).
func (c *Client) GetBalance(addr string) (string, error) {
	var result string
	err := c.call("dsn_getBalance", []interface{}{addr}, &result)
	return result, err
}

// GetStateRoot returns the current state root hash.
func (c *Client) GetStateRoot() (string, error) {
	var result string
	err := c.call("dsn_getStateRoot", nil, &result)
	return result, err
}

// GetPendingTxs returns pending transactions from the mempool.
func (c *Client) GetPendingTxs() ([]types.Transaction, error) {
	var result []types.Transaction
	err := c.call("dsn_getPendingTxs", nil, &result)
	return result, err
}

// GetAccount returns full account details.
func (c *Client) GetAccount(addr string) (*AccountResult, error) {
	var result AccountResult
	err := c.call("dsn_getAccount", []interface{}{addr}, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// SendTransaction submits a transaction to the node.
func (c *Client) SendTransaction(tx *types.Transaction) (string, error) {
	var result string
	err := c.call("dsn_sendTransaction", []interface{}{tx}, &result)
	return result, err
}

// AccountResult mirrors the JSON-RPC account response.
type AccountResult struct {
	Address string `json:"address"`
	Balance string `json:"balance"`
	Nonce   uint64 `json:"nonce"`
}
