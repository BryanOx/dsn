package middleware

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
)

// JSONRPCError represents a JSON-RPC error response.
type JSONRPCError struct {
	JSONRPC string      `json:"jsonrpc"`
	Error   *RPCErr     `json:"error"`
	ID      interface{} `json:"id"`
}

// RPCErr represents a JSON-RPC error object.
type RPCErr struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// JSON-RPC error codes
const (
	ParseError     = -32700
	InvalidRequest = -32600
	MethodNotFound = -32601
	InvalidParams  = -32602
	InternalError  = -32603
)

// ValidationMiddleware validates JSON-RPC requests.
func ValidationMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only validate POST requests with JSON content
		if r.Method == http.MethodPost {
			contentType := r.Header.Get("Content-Type")
			if contentType != "" && contentType != "application/json" {
				writeJSONRPCError(w, InvalidRequest, "content-type must be application/json", nil)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// RPCRequestFields represents the fields we validate from a parsed request.
type RPCRequestFields struct {
	JSONRPC string
	Method  string
}

// ValidateRequest validates a parsed RPC request.
func ValidateRequest(fields RPCRequestFields) *RPCErr {
	// Check JSON-RPC version
	if fields.JSONRPC != "2.0" {
		return &RPCErr{Code: InvalidRequest, Message: "invalid jsonrpc version, must be 2.0"}
	}

	// Check method is non-empty
	if fields.Method == "" {
		return &RPCErr{Code: InvalidRequest, Message: "method is required"}
	}

	return nil
}

// ValidateBatchRequest checks if the request is a batch and rejects it for v1.
func ValidateBatchRequest(data []byte) *RPCErr {
	// Trim whitespace
	for len(data) > 0 && (data[0] == ' ' || data[0] == '\n' || data[0] == '\t' || data[0] == '\r') {
		data = data[1:]
	}

	// If starts with '[', it's a batch request - reject in v1
	if len(data) > 0 && data[0] == '[' {
		return &RPCErr{Code: InvalidRequest, Message: "batch requests not supported in v1"}
	}

	return nil
}

// ValidateHexString validates a hex string input.
// Returns an error if the string is not a valid hex string.
func ValidateHexString(s string, maxLen int) *RPCErr {
	if s == "" {
		return nil // Empty is allowed for optional fields
	}

	// Check for 0x prefix
	if len(s) >= 2 && s[0:2] == "0x" {
		s = s[2:]
	}

	// Check length
	if len(s) > maxLen {
		return &RPCErr{Code: InvalidParams, Message: "hex string exceeds maximum length"}
	}

	// Check for valid hex characters
	if !isValidHex(s) {
		return &RPCErr{Code: InvalidParams, Message: "invalid hex string: contains non-hex characters"}
	}

	return nil
}

// isValidHex checks if a string contains only valid hex characters.
func isValidHex(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// MaxHexLength constants for different use cases
const (
	MaxAddressHexLength = 40          // 20 bytes = 40 hex chars
	MaxTransactionHash  = 64          // 32 bytes = 64 hex chars
	MaxDataHexLength    = 1024 * 1024 // 1 MiB of hex = 2 MiB of bytes
)

// ValidateAddress validates a DSN address string.
// DSN addresses should be 0x prefixed and followed by 40 hex characters (20 bytes).
var addressRegex = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)

// ValidateAddress validates that an address string is properly formatted.
func ValidateAddress(addr string) *RPCErr {
	if addr == "" {
		return &RPCErr{Code: InvalidParams, Message: "address is required"}
	}

	if !addressRegex.MatchString(addr) {
		return &RPCErr{Code: InvalidParams, Message: "invalid address format: must be 0x followed by 40 hex characters"}
	}

	return nil
}

// ValidateBlockHash validates a block hash string (64 hex chars).
func ValidateBlockHash(hash string) *RPCErr {
	if hash == "" {
		return nil // Optional field
	}

	if len(hash) != 64 {
		return &RPCErr{Code: InvalidParams, Message: "invalid block hash: must be 64 hex characters"}
	}

	if !isValidHex(hash) {
		return &RPCErr{Code: InvalidParams, Message: "invalid block hash: contains non-hex characters"}
	}

	return nil
}

// ValidateTransactionHash validates a transaction hash string.
func ValidateTransactionHash(hash string) *RPCErr {
	if hash == "" {
		return nil // Optional field
	}

	if len(hash) != 64 && len(hash) != 66 { // 0x prefix optional
		return &RPCErr{Code: InvalidParams, Message: "invalid transaction hash: must be 64 hex characters"}
	}

	// Check hex validity
	err := ValidateHexString(hash, MaxTransactionHash)
	if err != nil {
		return err
	}

	return nil
}

// ValidateGasLimit validates the gas limit for callContract.
// Enforces maximum gas limit of 1,000,000 per SC-SEC-006.
const MaxGasLimit = 1000000

// ValidateGasLimit validates that a gas limit is within acceptable bounds.
func ValidateGasLimit(gasLimit uint64) *RPCErr {
	if gasLimit > MaxGasLimit {
		return &RPCErr{Code: InvalidParams, Message: "gas limit exceeds maximum (1,000,000)"}
	}
	return nil
}

// ParseAndValidateHex parses a hex string and validates it.
// Returns the decoded bytes and any error.
func ParseAndValidateHex(s string, maxLen int) ([]byte, *RPCErr) {
	if s == "" {
		return nil, nil
	}

	// Check length
	if len(s) > maxLen {
		return nil, &RPCErr{Code: InvalidParams, Message: "hex string exceeds maximum length"}
	}

	// Strip 0x prefix if present
	if len(s) >= 2 && s[0:2] == "0x" {
		s = s[2:]
	}

	// Check for valid hex
	if !isValidHex(s) {
		return nil, &RPCErr{Code: InvalidParams, Message: "invalid hex string: contains non-hex characters"}
	}

	// Decode
	data, err := hex.DecodeString(s)
	if err != nil {
		return nil, &RPCErr{Code: InvalidParams, Message: "invalid hex string: " + err.Error()}
	}

	return data, nil
}

// NormalizeHex normalizes a hex string by ensuring it has 0x prefix.
func NormalizeHex(s string) string {
	if s == "" {
		return ""
	}
	// Handle both 0x and 0X prefixes
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		return "0x" + s[2:]
	}
	return "0x" + s
}

func writeJSONRPCError(w http.ResponseWriter, code int, message string, id interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(JSONRPCError{
		JSONRPC: "2.0",
		Error:   &RPCErr{Code: code, Message: message},
		ID:      id,
	})
}
