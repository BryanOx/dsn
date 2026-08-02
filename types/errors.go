package types

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidAddress   = errors.New("invalid address length")
	ErrAmountOverflow   = errors.New("amount overflow")
	ErrAmountUnderflow  = errors.New("amount underflow")
	ErrInvalidSignature = errors.New("invalid signature")
	ErrNonceMismatch    = errors.New("nonce mismatch")
	ErrDuplicateIntent  = errors.New("duplicate intent")
	ErrMaxFeeExceeded   = errors.New("max fee exceeded")
	ErrInvalidTimestamp = errors.New("invalid timestamp")
	ErrNegativeBalance  = errors.New("negative balance not allowed")
	ErrInvalidEncoding  = errors.New("invalid binary encoding")
	ErrPayloadTooLarge  = errors.New("payload too large")
	// Evidence errors
	ErrInvalidEvidenceType = errors.New("invalid evidence type")
	ErrEvidenceMismatch    = errors.New("evidence hash mismatch")
	ErrDuplicateEvidence   = errors.New("duplicate evidence")
	// Contract errors
	ErrContractNotFound      = errors.New("contract not found")
	ErrGasLimitExceeded      = errors.New("gas limit exceeded")
	ErrContractSizeExceeded  = errors.New("contract size exceeds maximum")
	ErrCallDepthExceeded     = errors.New("call depth limit exceeded")
	ErrUnknownHostImport     = errors.New("unknown host function import")
	ErrContractRevert        = errors.New("contract execution reverted")
	ErrInvalidContractID     = errors.New("invalid contract identifier")
	ErrContractAlreadyExists = errors.New("contract already exists at this ID")
)

// ZeroHash is a preallocated zero value for Hash type
var ZeroHash = Hash{}

type ValidationError struct {
	Err   error
	Field string
	Value interface{}
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error on %s: %v", e.Field, e.Err)
}

func (e *ValidationError) Unwrap() error {
	return e.Err
}
