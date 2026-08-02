package types

import (
	"math"
	"math/big"
	"testing"
)

func TestNewAmount(t *testing.T) {
	tests := []struct {
		input    uint64
		wantZero bool
	}{
		{0, true},
		{100, false},
		{math.MaxUint64, false},
	}

	for _, tt := range tests {
		a := NewAmount(tt.input)
		if tt.wantZero && !a.IsZero() {
			t.Errorf("NewAmount(%d).IsZero() = false, want true", tt.input)
		}
		if !tt.wantZero && a.IsZero() {
			t.Errorf("NewAmount(%d).IsZero() = true, want false", tt.input)
		}
	}
}

func TestAmountAdd(t *testing.T) {
	// Test valid addition
	a := NewAmount(100)
	b := NewAmount(50)
	result, err := a.Add(b)
	if err != nil {
		t.Errorf("Add() error = %v", err)
	}
	if result.Cmp(NewAmount(150)) != 0 {
		t.Errorf("Add result = %v, want 150", result)
	}

	// Test overflow (max 128-bit)
	// Max uint128 = 2^128 - 1
	maxVal := new(big.Int).Sub(new(big.Int).Lsh(new(big.Int).SetUint64(1), 128), new(big.Int).SetUint64(1))
	maxAmount := Amount{val: maxVal}
	_, err = maxAmount.Add(NewAmount(1))
	if err != ErrAmountOverflow {
		t.Errorf("Add overflow: got %v, want ErrAmountOverflow", err)
	}
}

func TestAmountSub(t *testing.T) {
	// Test valid subtraction
	a := NewAmount(100)
	b := NewAmount(50)
	result, err := a.Sub(b)
	if err != nil {
		t.Errorf("Sub() error = %v", err)
	}
	if result.Cmp(NewAmount(50)) != 0 {
		t.Errorf("Sub result = %v, want 50", result)
	}

	// Test underflow
	c := NewAmount(50)
	_, err = c.Sub(NewAmount(100))
	if err != ErrAmountUnderflow {
		t.Errorf("Sub underflow: got %v, want ErrAmountUnderflow", err)
	}
}

func TestAmountCmp(t *testing.T) {
	a := NewAmount(100)
	b := NewAmount(50)
	c := NewAmount(100)

	if a.Cmp(b) != 1 {
		t.Error("100 should be > 50")
	}
	if b.Cmp(a) != -1 {
		t.Error("50 should be < 100")
	}
	if a.Cmp(c) != 0 {
		t.Error("100 should be == 100")
	}
}

func TestAmountIsZero(t *testing.T) {
	if !NewAmount(0).IsZero() {
		t.Error("0 should be zero")
	}
	if NewAmount(100).IsZero() {
		t.Error("100 should not be zero")
	}
}

func TestAmountMarshalUnmarshal(t *testing.T) {
	original := NewAmount(123456789)

	data, err := original.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary() error = %v", err)
	}

	var restored Amount
	err = restored.UnmarshalBinary(data)
	if err != nil {
		t.Fatalf("UnmarshalBinary() error = %v", err)
	}

	if original.Cmp(restored) != 0 {
		t.Errorf("round-trip: got %v, want %v", restored, original)
	}
}
