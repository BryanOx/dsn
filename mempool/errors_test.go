package mempool

import (
	"errors"
	"testing"
)

func TestMempoolErrors(t *testing.T) {
	if !errors.Is(ErrMempoolFull, ErrMempoolFull) {
		t.Error("ErrMempoolFull should identify itself")
	}
	if !errors.Is(ErrTxExpired, ErrTxExpired) {
		t.Error("ErrTxExpired should identify itself")
	}
	if !errors.Is(ErrAlreadyInPool, ErrAlreadyInPool) {
		t.Error("ErrAlreadyInPool should identify itself")
	}
}

func TestMempoolErrorsDistinct(t *testing.T) {
	if errors.Is(ErrMempoolFull, ErrTxExpired) {
		t.Error("errors should be distinct")
	}
	if errors.Is(ErrMempoolFull, ErrAlreadyInPool) {
		t.Error("errors should be distinct")
	}
	if errors.Is(ErrTxExpired, ErrAlreadyInPool) {
		t.Error("errors should be distinct")
	}
}
