package mempool

import "errors"

var (
	ErrMempoolFull              = errors.New("mempool is full")
	ErrTxExpired                = errors.New("transaction expired")
	ErrAlreadyInPool            = errors.New("transaction already in pool")
	ErrValidatorAlreadyRegistered = errors.New("validator already registered")
	ErrInsufficientStake        = errors.New("insufficient stake for validator registration")
)