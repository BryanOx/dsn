package rpc

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dsn/dsn/node"
)

func TestNewServer(t *testing.T) {
	n, _ := node.New(node.DefaultConfig())
	s := New(n)
	if s == nil {
		t.Fatal("server is nil")
	}
}

func TestRPCMethodNotFound(t *testing.T) {
	n, _ := node.New(node.DefaultConfig())
	s := New(n)

	body := `{"jsonrpc":"2.0","method":"dsn_nonexistent","params":[],"id":1}`
	req := httptest.NewRequest("POST", "/", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	s.ServeHTTP(w, req)

	var resp RPCResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error == nil {
		t.Fatal("expected error for nonexistent method")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("error code = %d, want -32601", resp.Error.Code)
	}
}

func TestRPCGetStateRoot(t *testing.T) {
	n, _ := node.New(node.DefaultConfig())
	s := New(n)

	body := `{"jsonrpc":"2.0","method":"dsn_getStateRoot","params":[],"id":1}`
	req := httptest.NewRequest("POST", "/", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	s.ServeHTTP(w, req)

	var resp RPCResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatal("result should be an object")
	}
	if _, ok := result["state_root"]; !ok {
		t.Error("result should contain state_root")
	}
}

func TestRPCGetBalance(t *testing.T) {
	n, _ := node.New(node.DefaultConfig())
	s := New(n)

	body := `{"jsonrpc":"2.0","method":"dsn_getBalance","params":{"address":"1Fu5U5akQPbnajRcAvdkkdqCGquNAJegRM"},"id":1}`
	req := httptest.NewRequest("POST", "/", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	s.ServeHTTP(w, req)

	var resp RPCResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatal("result should be an object")
	}
	if _, ok := result["balance"]; !ok {
		t.Error("result should contain balance")
	}
}

func TestRPCHTTPInvalidMethod(t *testing.T) {
	n, _ := node.New(node.DefaultConfig())
	s := New(n)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	s.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var resp RPCResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error == nil {
		t.Fatal("expected error for GET request")
	}
}

func TestRPCHTTPServer(t *testing.T) {
	n, _ := node.New(node.DefaultConfig())
	s := New(n)

	body := `{"jsonrpc":"2.0","method":"dsn_getStateRoot","params":[],"id":1}`
	req := httptest.NewRequest("POST", "/", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	s.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var resp RPCResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error != nil {
		t.Fatalf("error: %v", resp.Error)
	}
}

func TestRPCInvalidJSON(t *testing.T) {
	n, _ := node.New(node.DefaultConfig())
	s := New(n)

	body := `not json at all`
	req := httptest.NewRequest("POST", "/", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	s.ServeHTTP(w, req)

	var resp RPCResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if resp.Error.Code != -32700 {
		t.Errorf("error code = %d, want -32700", resp.Error.Code)
	}
}

func TestRPCGetAccount(t *testing.T) {
	n, _ := node.New(node.DefaultConfig())
	s := New(n)

	body := `{"jsonrpc":"2.0","method":"dsn_getAccount","params":{"address":"1Fu5U5akQPbnajRcAvdkkdqCGquNAJegRM"},"id":1}`
	req := httptest.NewRequest("POST", "/", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	s.ServeHTTP(w, req)

	var resp RPCResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
}

func TestRPCGetPendingTxs(t *testing.T) {
	n, _ := node.New(node.DefaultConfig())
	s := New(n)

	body := `{"jsonrpc":"2.0","method":"dsn_getPendingTxs","params":[],"id":1}`
	req := httptest.NewRequest("POST", "/", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	s.ServeHTTP(w, req)

	var resp RPCResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
}
