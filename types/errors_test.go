package types

import (
	"errors"
	"testing"
)

func TestSentinelErrors(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		target   error
		want     bool
	}{
		{"ErrInvalidAddress", ErrInvalidAddress, ErrInvalidAddress, true},
		{"ErrAmountOverflow", ErrAmountOverflow, ErrAmountOverflow, true},
		{"ErrAmountUnderflow", ErrAmountUnderflow, ErrAmountUnderflow, true},
		{"ErrInvalidSignature", ErrInvalidSignature, ErrInvalidSignature, true},
		{"ErrNonceMismatch", ErrNonceMismatch, ErrNonceMismatch, true},
		{"ErrDuplicateIntent", ErrDuplicateIntent, ErrDuplicateIntent, true},
		{"ErrMaxFeeExceeded", ErrMaxFeeExceeded, ErrMaxFeeExceeded, true},
		{"ErrInvalidTimestamp", ErrInvalidTimestamp, ErrInvalidTimestamp, true},
		{"ErrNegativeBalance", ErrNegativeBalance, ErrNegativeBalance, true},
		{"ErrInvalidEncoding", ErrInvalidEncoding, ErrInvalidEncoding, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := errors.Is(tt.err, tt.target)
			if got != tt.want {
				t.Errorf("errors.Is(%v, %v) = %v, want %v", tt.err, tt.target, got, tt.want)
			}
		})
	}
}

func TestValidationErrorUnwrap(t *testing.T) {
	ve := &ValidationError{
		Err:   ErrInvalidAddress,
		Field: "address",
		Value: []byte{1, 2, 3},
	}

	// Test that errors.Is works through ValidationError
	if !errors.Is(ve, ErrInvalidAddress) {
		t.Error("errors.Is should unwrap ValidationError to ErrInvalidAddress")
	}
}

func TestValidationErrorError(t *testing.T) {
	ve := &ValidationError{
		Err:   ErrAmountOverflow,
		Field: "amount",
		Value: "9999999999999999999999999999999999999999999999999999999999999999",
	}

	errMsg := ve.Error()

	if errMsg == "" {
		t.Error("ValidationError.Error() should not return empty string")
	}

	// Should contain field name
	if !contains(errMsg, "amount") {
		t.Errorf("Error message should contain field name 'amount', got: %s", errMsg)
	}

	// Should contain error description
	if !contains(errMsg, "amount overflow") && !contains(errMsg, "AmountOverflow") {
		t.Errorf("Error message should contain error description, got: %s", errMsg)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && (s[:len(substr)] == substr || contains(s[1:], substr)))
}