package types

import (
	"math/big"
)

// Amount represents a non-negative 128-bit unsigned integer
type Amount struct {
	val *big.Int
}

// NewAmount creates an Amount from a uint64
func NewAmount(v uint64) Amount {
	return Amount{val: new(big.Int).SetUint64(v)}
}

// Add adds two amounts, returning error on overflow (> 128 bits)
func (a Amount) Add(b Amount) (Amount, error) {
	res := new(big.Int).Add(a.val, b.val)
	// Check overflow: bit length > 128
	if res.BitLen() > 128 {
		return Amount{}, ErrAmountOverflow
	}
	return Amount{val: res}, nil
}

// Sub subtracts two amounts, returning error on underflow (negative result)
func (a Amount) Sub(b Amount) (Amount, error) {
	if a.val.Cmp(b.val) < 0 {
		return Amount{}, ErrAmountUnderflow
	}
	res := new(big.Int).Sub(a.val, b.val)
	return Amount{val: res}, nil
}

// IsZero returns true if the amount is zero
func (a Amount) IsZero() bool {
	return a.val.Sign() == 0
}

// Cmp compares two amounts: -1 if a < b, 0 if a == b, 1 if a > b
func (a Amount) Cmp(b Amount) int {
	return a.val.Cmp(b.val)
}

// MarshalBinary encodes the amount to bytes
func (a Amount) MarshalBinary() ([]byte, error) {
	return a.val.Bytes(), nil
}

// UnmarshalBinary decodes bytes to an amount
func (a *Amount) UnmarshalBinary(data []byte) error {
	// Limit to 16 bytes (128 bits max)
	if len(data) > 16 {
		return ErrAmountOverflow
	}
	a.val = new(big.Int).SetBytes(data)
	return nil
}

// String returns a decimal string representation
func (a Amount) String() string {
	return a.val.String()
}