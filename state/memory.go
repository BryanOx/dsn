package state

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"sort"
	"sync"

	"github.com/dsn/dsn/types"
)

var _ StateDB = (*InMemoryState)(nil)

// snapshot holds a deep copy of the state at a point in time
type snapshot struct {
	accounts map[types.Address]*Account
	kvstore  map[string][]byte
}

// StateDB defines the interface for state storage
type StateDB interface {
	GetAccount(addr types.Address) (*Account, error)
	SetAccount(addr types.Address, account *Account) error
	DeleteAccount(addr types.Address) error
	Commit() (types.Hash, error)
	GetStateRoot() types.Hash

	// Contract storage
	GetCode(contractID types.Hash) ([]byte, error)
	SetCode(contractID types.Hash, code []byte) error
	GetContractMeta(contractID types.Hash) (*types.ContractMetadata, error)
	SetContractMeta(contractID types.Hash, meta *types.ContractMetadata) error
	GetContractStorage(contractID types.Hash, key []byte) ([]byte, error)
	SetContractStorage(contractID types.Hash, key []byte, value []byte) error

	// State rollback support
	Snapshot() int
	RevertToSnapshot(int) error
}

// InMemoryState implements StateDB using an in-memory map.
// In addition to accounts, it holds a generic key-value store (kvstore) for
// non-account state such as the on-chain validator registry, staking ledger,
// and epoch data. Both accounts and kvstore entries are included in the SMT
// state root during Commit().
type InMemoryState struct {
	mu        sync.RWMutex
	accounts  map[types.Address]*Account
	kvstore   map[string][]byte
	smt       *SMT
	hasher    types.Hasher
	snapshots []snapshot
}

func NewInMemoryState(hasher types.Hasher) *InMemoryState {
	return &InMemoryState{
		accounts: make(map[types.Address]*Account),
		kvstore:  make(map[string][]byte),
		smt:      NewSMT(hasher),
		hasher:   hasher,
	}
}

func (s *InMemoryState) GetAccount(addr types.Address) (*Account, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	acc, ok := s.accounts[addr]
	if !ok {
		return nil, fmt.Errorf("account not found: %s", addr.String())
	}
	return acc, nil
}

func (s *InMemoryState) SetAccount(addr types.Address, account *Account) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.accounts[addr] = account
	return nil
}

func (s *InMemoryState) DeleteAccount(addr types.Address) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.accounts, addr)
	return nil
}

// GetBytes retrieves a value from the generic kvstore by key.
// Returns nil, false if the key does not exist.
func (s *InMemoryState) GetBytes(key string) ([]byte, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	val, ok := s.kvstore[key]
	return val, ok
}

// SetBytes stores a value in the generic kvstore.
func (s *InMemoryState) SetBytes(key string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.kvstore[key] = value
	return nil
}

// DeleteBytes removes a key from the generic kvstore.
func (s *InMemoryState) DeleteBytes(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.kvstore, key)
	return nil
}

// ForEachAccount iterates over all accounts in memory.
// Returns the first error from fn, if any.
func (s *InMemoryState) ForEachAccount(fn func(types.Address, *Account) error) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for addr, acc := range s.accounts {
		if err := fn(addr, acc); err != nil {
			return err
		}
	}
	return nil
}

// ForEachKV iterates over all kvstore entries in memory.
// Returns the first error from fn, if any.
func (s *InMemoryState) ForEachKV(fn func(string, []byte) error) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for k, v := range s.kvstore {
		if err := fn(k, v); err != nil {
			return err
		}
	}
	return nil
}

func (s *InMemoryState) Commit() (types.Hash, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Rebuild SMT from accounts (sorted for deterministic ordering)
	s.smt = NewSMT(s.hasher)

	// Collect and sort addresses for deterministic iteration
	addrs := make([]types.Address, 0, len(s.accounts))
	for addr := range s.accounts {
		addrs = append(addrs, addr)
	}
	sort.Slice(addrs, func(i, j int) bool {
		return bytes.Compare(addrs[i][:], addrs[j][:]) < 0
	})

	for _, addr := range addrs {
		acc := s.accounts[addr]
		var buf bytes.Buffer
		if err := acc.Encode(&buf); err != nil {
			return types.Hash{}, err
		}
		if err := s.smt.Insert(addr.Bytes(), buf.Bytes()); err != nil {
			return types.Hash{}, err
		}
	}

	// Insert kvstore entries in sorted key order for deterministic state root
	kvKeys := make([]string, 0, len(s.kvstore))
	for k := range s.kvstore {
		kvKeys = append(kvKeys, k)
	}
	sort.Strings(kvKeys)
	for _, k := range kvKeys {
		val := s.kvstore[k]
		// Make a copy to avoid aliasing the map's internal storage
		valCopy := make([]byte, len(val))
		copy(valCopy, val)
		if err := s.smt.Insert([]byte(k), valCopy); err != nil {
			return types.Hash{}, err
		}
	}

	return s.smt.Root(), nil
}

func (s *InMemoryState) GetStateRoot() types.Hash {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.smt.Root()
}

// GetCode retrieves contract bytecode by contract ID.
func (s *InMemoryState) GetCode(contractID types.Hash) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := "code:" + fmt.Sprintf("%x", contractID[:])
	val, ok := s.kvstore[key]
	if !ok {
		return nil, types.ErrContractNotFound
	}
	return val, nil
}

// SetCode stores contract bytecode by contract ID.
func (s *InMemoryState) SetCode(contractID types.Hash, code []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := "code:" + fmt.Sprintf("%x", contractID[:])
	s.kvstore[key] = code
	return nil
}

// GetContractMeta retrieves contract metadata by contract ID.
func (s *InMemoryState) GetContractMeta(contractID types.Hash) (*types.ContractMetadata, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := "meta:" + fmt.Sprintf("%x", contractID[:])
	val, ok := s.kvstore[key]
	if !ok {
		return nil, types.ErrContractNotFound
	}
	meta := &types.ContractMetadata{}
	if err := meta.Decode(bytes.NewReader(val)); err != nil {
		return nil, err
	}
	return meta, nil
}

// SetContractMeta stores contract metadata by contract ID.
func (s *InMemoryState) SetContractMeta(contractID types.Hash, meta *types.ContractMetadata) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := "meta:" + fmt.Sprintf("%x", contractID[:])
	var buf bytes.Buffer
	if err := meta.Encode(&buf); err != nil {
		return err
	}
	s.kvstore[key] = buf.Bytes()
	return nil
}

// GetContractStorage retrieves a value from contract storage.
func (s *InMemoryState) GetContractStorage(contractID types.Hash, key []byte) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Hash the storage key to avoid long keys in kvstore
	keyHash := sha256.Sum256(key)
	storageKey := "storage:" + fmt.Sprintf("%x", contractID[:]) + ":" + fmt.Sprintf("%x", keyHash[:])
	val, ok := s.kvstore[storageKey]
	if !ok {
		return nil, fmt.Errorf("storage key not found")
	}
	return val, nil
}

// SetContractStorage stores a value in contract storage.
func (s *InMemoryState) SetContractStorage(contractID types.Hash, key []byte, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(value) > 64*1024 {
		return fmt.Errorf("storage value exceeds 64 KiB limit")
	}
	// Hash the storage key to avoid long keys in kvstore
	keyHash := sha256.Sum256(key)
	storageKey := "storage:" + fmt.Sprintf("%x", contractID[:]) + ":" + fmt.Sprintf("%x", keyHash[:])
	s.kvstore[storageKey] = value
	return nil
}

// Snapshot saves a snapshot of the current state and returns a revision ID.
func (s *InMemoryState) Snapshot() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	snap := snapshot{
		accounts: copyAccounts(s.accounts),
		kvstore:  copyKV(s.kvstore),
	}
	s.snapshots = append(s.snapshots, snap)
	return len(s.snapshots) - 1
}

// RevertToSnapshot reverts the state to a previous snapshot.
func (s *InMemoryState) RevertToSnapshot(id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if id < 0 || id >= len(s.snapshots) {
		return ErrInvalidSnapshot
	}
	snap := s.snapshots[id]
	s.accounts = copyAccounts(snap.accounts)
	s.kvstore = copyKV(snap.kvstore)
	s.snapshots = s.snapshots[:id]
	return nil
}

// copyAccounts creates a deep copy of the accounts map
func copyAccounts(src map[types.Address]*Account) map[types.Address]*Account {
	dst := make(map[types.Address]*Account, len(src))
	for k, v := range src {
		vCopy := *v
		dst[k] = &vCopy
	}
	return dst
}

// copyKV creates a deep copy of the kvstore map
func copyKV(src map[string][]byte) map[string][]byte {
	dst := make(map[string][]byte, len(src))
	for k, v := range src {
		vCopy := make([]byte, len(v))
		copy(vCopy, v)
		dst[k] = vCopy
	}
	return dst
}