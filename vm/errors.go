package vm

import "errors"

var (
	ErrExecutionReverted    = errors.New("execution reverted")
	ErrGasLimitExceeded     = errors.New("gas limit exceeded")
	ErrContractNotFound     = errors.New("contract not found")
	ErrCallDepthExceeded    = errors.New("call depth exceeded")
	ErrCodeTooLarge         = errors.New("contract bytecode too large")
	ErrInvalidWasm          = errors.New("invalid WASM module")
	ErrHostFunction         = errors.New("host function error")
	ErrStorageValueTooLarge = errors.New("storage value exceeds maximum size")
	ErrStorageKeyTooLarge   = errors.New("storage key exceeds maximum size")
	ErrInvalidImport        = errors.New("invalid import: only 'env' module with allowed host functions is permitted")
)