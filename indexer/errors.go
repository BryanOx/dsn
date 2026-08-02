package indexer

import (
	"errors"
	"fmt"
)

// ErrIndexerNotAvailable is returned when the indexer is disabled or not available.
var ErrIndexerNotAvailable = errors.New("indexer not available")

// IndexerError represents an indexer-specific error.
type IndexerError struct {
	Code    int64
	Message string
	Err     error
}

func (e *IndexerError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *IndexerError) Unwrap() error {
	return e.Err
}

// NewIndexerError creates a new indexer error with the specified code and message.
func NewIndexerError(code int64, message string, err error) *IndexerError {
	return &IndexerError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

// ErrCode returns the JSON-RPC error code.
func (e *IndexerError) ErrCode() int64 {
	return e.Code
}

// ErrorCode constants for indexer-specific errors.
const (
	ErrCodeIndexerNotAvailable = -32001
)

// NewErrIndexerNotAvailable creates an error for when the indexer is not available.
func NewErrIndexerNotAvailable(err error) *IndexerError {
	return NewIndexerError(ErrCodeIndexerNotAvailable, "indexer not available", err)
}
