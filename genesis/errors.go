package genesis

import "fmt"

// GenesisError is a genesis-related error with a code, message, and optional cause.
type GenesisError struct {
	Code    string
	Message string
	Cause   error
}

func (e *GenesisError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *GenesisError) Unwrap() error {
	return e.Cause
}

func (e *GenesisError) Wrap(err error) *GenesisError {
	return &GenesisError{
		Code:    e.Code,
		Message: e.Message,
		Cause:   err,
	}
}

// Error codes
const (
	ErrCodePathRequired     = "GENESIS_PATH_REQUIRED"
	ErrCodeFileNotFound     = "GENESIS_FILE_NOT_FOUND"
	ErrCodeReadFailed       = "GENESIS_READ_FAILED"
	ErrCodeParseFailed      = "GENESIS_PARSE_FAILED"
	ErrCodeMissingChainID   = "GENESIS_MISSING_CHAIN_ID"
	ErrCodeMissingGenesisTime = "GENESIS_MISSING_GENESIS_TIME"
	ErrCodeHashFailed       = "GENESIS_HASH_FAILED"
	ErrCodeValidationFailed = "GENESIS_VALIDATION_FAILED"
)

// Predefined genesis errors
var (
	ErrGenesisPathRequired = &GenesisError{
		Code:    ErrCodePathRequired,
		Message: "genesis file path is required",
	}
	ErrGenesisFileNotFound = &GenesisError{
		Code:    ErrCodeFileNotFound,
		Message: "genesis file not found",
	}
	ErrGenesisReadFailed = &GenesisError{
		Code:    ErrCodeReadFailed,
		Message: "failed to read genesis file",
	}
	ErrGenesisParseFailed = &GenesisError{
		Code:    ErrCodeParseFailed,
		Message: "failed to parse genesis file",
	}
	ErrGenesisMissingChainID = &GenesisError{
		Code:    ErrCodeMissingChainID,
		Message: "missing required field: chain_id",
	}
	ErrGenesisMissingGenesisTime = &GenesisError{
		Code:    ErrCodeMissingGenesisTime,
		Message: "missing required field: genesis_time",
	}
	ErrGenesisHashFailed = &GenesisError{
		Code:    ErrCodeHashFailed,
		Message: "failed to compute genesis hash",
	}
	ErrGenesisValidationFailed = &GenesisError{
		Code:    ErrCodeValidationFailed,
		Message: "genesis validation failed",
	}
)