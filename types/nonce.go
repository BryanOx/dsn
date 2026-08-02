package types

// Nonce represents a transaction sequence number for an account
type Nonce uint64

// Next returns the next nonce value
func (n Nonce) Next() Nonce {
	return n + 1
}
