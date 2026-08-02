package types

// Hash is a 32-byte identifier for cryptographic digests
type Hash [32]byte

// Bytes returns the raw hash bytes
func (h Hash) Bytes() []byte {
	return h[:]
}

// IsZero returns true if the hash is all zeros
func (h Hash) IsZero() bool {
	for _, b := range h {
		if b != 0 {
			return false
		}
	}
	return true
}
