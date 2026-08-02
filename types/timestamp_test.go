package types

import (
	"testing"
	"time"
)

func TestValidateTimestamp(t *testing.T) {
	// Current timestamp should pass
	now := uint64(time.Now().Unix())
	if err := ValidateTimestamp(now); err != nil {
		t.Errorf("Current timestamp should pass: %v", err)
	}

	// Future within 5s should pass
	future := uint64(time.Now().Unix() + 4)
	if err := ValidateTimestamp(future); err != nil {
		t.Errorf("Future timestamp within 5s should pass: %v", err)
	}

	// Future too far should fail
	tooFarFuture := uint64(time.Now().Unix() + 10)
	if err := ValidateTimestamp(tooFarFuture); err == nil {
		t.Error("Future timestamp > 5s should fail")
	}

	// Past within 5s should pass
	past := uint64(time.Now().Unix() - 4)
	if err := ValidateTimestamp(past); err != nil {
		t.Errorf("Past timestamp within 5s should pass: %v", err)
	}

	// Past too far should fail
	tooFarPast := uint64(time.Now().Unix() - 10)
	if err := ValidateTimestamp(tooFarPast); err == nil {
		t.Error("Past timestamp > 5s should fail")
	}
}
