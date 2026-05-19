package vm

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/dsn/dsn/state"
	"github.com/dsn/dsn/types"
)

const (
	// MaxStorageKeySize is the maximum allowed size for a storage key.
	MaxStorageKeySize = 256
	// MaxStorageValueSize is the maximum allowed size for a storage value (64 KiB).
	MaxStorageValueSize = 64 * 1024
)

// ContractStore provides SMT-backed key/value storage for contracts.
// It wraps a state.StateDB and provides contract-specific storage operations.
type ContractStore struct {
	st         state.StateDB
	contractID types.Hash
}

// NewContractStore creates a new ContractStore for a specific contract.
func NewContractStore(st state.StateDB, contractID types.Hash) *ContractStore {
	return &ContractStore{st: st, contractID: contractID}
}

// Read reads a value from contract storage.
func (cs *ContractStore) Read(key []byte) ([]byte, error) {
	if len(key) > MaxStorageKeySize {
		return nil, ErrStorageKeyTooLarge
	}
	return cs.st.GetContractStorage(cs.contractID, key)
}

// Write writes a value to contract storage.
func (cs *ContractStore) Write(key []byte, value []byte) error {
	if len(key) > MaxStorageKeySize {
		return ErrStorageKeyTooLarge
	}
	if len(value) > MaxStorageValueSize {
		return ErrStorageValueTooLarge
	}
	return cs.st.SetContractStorage(cs.contractID, key, value)
}

// ContractStoreBridge adapts state.StateDB to a simpler ContractStore interface.
// This is useful for code that doesn't need the full StateDB interface.
type ContractStoreBridge struct {
	db     state.StateDB
	hasher types.Hasher
}

// NewContractStoreBridge creates a new bridge with the given state and hasher.
func NewContractStoreBridge(db state.StateDB, hasher types.Hasher) *ContractStoreBridge {
	return &ContractStoreBridge{db: db, hasher: hasher}
}

// keyForCode generates the storage key for contract code.
func (b *ContractStoreBridge) keyForCode(contractID types.Hash) string {
	return "code:" + hex.EncodeToString(contractID[:])
}

// keyForMeta generates the storage key for contract metadata.
func (b *ContractStoreBridge) keyForMeta(contractID types.Hash) string {
	return "meta:" + hex.EncodeToString(contractID[:])
}

// keyForStorage generates the storage key for contract storage.
// The key is hashed to avoid long keys in the kvstore.
func (b *ContractStoreBridge) keyForStorage(contractID types.Hash, key []byte) string {
	keyHash := sha256.Sum256(key)
	return "storage:" + hex.EncodeToString(contractID[:]) + ":" + hex.EncodeToString(keyHash[:])
}