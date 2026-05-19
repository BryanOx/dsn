package sdk

import (
	"errors"
	"fmt"
)

// SDK error types

var (
	// ErrNotFound is returned when a resource is not found.
	ErrNotFound = errors.New("not found")

	// ErrInvalidParams is returned when parameters are invalid.
	ErrInvalidParams = errors.New("invalid params")

	// ErrInternal is returned for internal errors.
	ErrInternal = errors.New("internal error")

	// ErrMethodNotFound is returned when the method doesn't exist.
	ErrMethodNotFound = errors.New("method not found")

	// ErrParse is returned for parse errors.
	ErrParse = errors.New("parse error")

	// ErrInvalidRequest is returned for invalid requests.
	ErrInvalidRequest = errors.New("invalid request")

	// ErrServerError is returned for server errors (-32000 to -32099).
	ErrServerError = errors.New("server error")

	// ErrIndexerNotAvailable is returned when indexer is not available.
	ErrIndexerNotAvailable = errors.New("indexer not available")

	// ErrConnection is returned for connection errors.
	ErrConnection = errors.New("connection error")

	// ErrTimeout is returned for timeout errors.
	ErrTimeout = errors.New("timeout")

	// ErrCancelled is returned when context is cancelled.
	ErrCancelled = errors.New("cancelled")

	// ErrRejected is returned when request is rejected.
	ErrRejected = errors.New("rejected")
)

// JSON-RPC error codes
const (
	JSONRPCParseError     = -32700
	JSONRPCInvalidRequest = -32600
	JSONRPCMethodNotFound = -32601
	JSONRPCInvalidParams  = -32602
	JSONRPCInternalError  = -32603

	// DSN-specific error codes (range -32000 to -32099)
	DSNServerError       = -32000
	DSNIndexerNotAvail   = -32001
	DSNInsufficientFunds = -32002
	DSNInvalidNonce      = -32003
	DSNGasTooLow         = -32004
	DSNGasTooHigh        = -32005
	DSNContractNotFound  = -32006
	DSNContractError     = -32007
)

// SDKError wraps a JSON-RPC error with additional context.
type SDKError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	err     error
}

// Error returns the error message.
func (e *SDKError) Error() string {
	if e.err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.err)
	}
	return e.Message
}

// Unwrap returns the underlying error.
func (e *SDKError) Unwrap() error {
	return e.err
}

// Is compares errors for equality.
func (e *SDKError) Is(target error) bool {
	if target == ErrNotFound {
		return e.Code == JSONRPCMethodNotFound || e.Code == DSNContractNotFound
	}
	if target == ErrInvalidParams {
		return e.Code == JSONRPCInvalidParams
	}
	if target == ErrInternal {
		return e.Code == JSONRPCInternalError || e.Code == DSNServerError
	}
	if target == ErrMethodNotFound {
		return e.Code == JSONRPCMethodNotFound
	}
	if target == ErrIndexerNotAvailable {
		return e.Code == DSNIndexerNotAvail
	}
	return false
}

// parseRPCError converts a JSON-RPC error to an SDK error.
func parseRPCError(rpcErr *RPCError) error {
	if rpcErr == nil {
		return nil
	}

	var sdkErr *SDKError
	switch rpcErr.Code {
	case JSONRPCParseError:
		sdkErr = &SDKError{Code: rpcErr.Code, Message: "Parse error", err: ErrParse}
	case JSONRPCInvalidRequest:
		sdkErr = &SDKError{Code: rpcErr.Code, Message: "Invalid Request", err: ErrInvalidRequest}
	case JSONRPCMethodNotFound:
		sdkErr = &SDKError{Code: rpcErr.Code, Message: "Method not found", err: ErrMethodNotFound}
	case JSONRPCInvalidParams:
		sdkErr = &SDKError{Code: rpcErr.Code, Message: "Invalid params", err: ErrInvalidParams}
	case JSONRPCInternalError:
		sdkErr = &SDKError{Code: rpcErr.Code, Message: "Internal error", err: ErrInternal}
	case DSNIndexerNotAvail:
		sdkErr = &SDKError{Code: rpcErr.Code, Message: "Indexer not available", err: ErrIndexerNotAvailable}
	case DSNInsufficientFunds:
		sdkErr = &SDKError{Code: rpcErr.Code, Message: rpcErr.Message, err: ErrServerError}
	case DSNInvalidNonce:
		sdkErr = &SDKError{Code: rpcErr.Code, Message: rpcErr.Message, err: ErrServerError}
	default:
		sdkErr = &SDKError{Code: rpcErr.Code, Message: rpcErr.Message, err: ErrServerError}
	}

	return sdkErr
}

// NewSDKError creates a new SDK error with code and message.
func NewSDKError(code int, message string) error {
	return &SDKError{
		Code:    code,
		Message: message,
		err:     ErrServerError,
	}
}

// IsNotFound checks if the error is a not found error.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// IsInvalidParams checks if the error is an invalid params error.
func IsInvalidParams(err error) bool {
	return errors.Is(err, ErrInvalidParams)
}

// IsInternal checks if the error is an internal error.
func IsInternal(err error) bool {
	return errors.Is(err, ErrInternal)
}

// IsIndexerNotAvailable checks if the error is an indexer not available error.
func IsIndexerNotAvailable(err error) bool {
	return errors.Is(err, ErrIndexerNotAvailable)
}