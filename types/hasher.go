package types

import (
	"crypto/sha256"
)

// Hasher defines the interface for computing cryptographic hashes
type Hasher interface {
	Hash(data []byte) (Hash, error)
}

// SHA256Hasher implements Hasher using SHA-256
type SHA256Hasher struct{}

// Hash computes SHA-256 of the input data
func (h SHA256Hasher) Hash(data []byte) (Hash, error) {
	return Hash(sha256.Sum256(data)), nil
}