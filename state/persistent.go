package state

import (
	"bytes"
	"fmt"
	"sort"
	"time"

	"github.com/BryanOx/dsn/types"
	"go.etcd.io/bbolt"
)

var (
	accountsBucket = []byte("accounts")
	kvstoreBucket  = []byte("kvstore")
	metaBucket     = []byte("meta")
	stateRootKey   = []byte("state_root")
)

// PersistentState implements StateDB using BoltDB for persistent storage.
// In addition to accounts, it persists kvstore entries for non-account state
// (validator registry, staking ledger, epochs, etc.).
// Pure Go, no CGO required.
type PersistentState struct {
	db      *bbolt.DB
	hasher  types.Hasher
	cache   map[types.Address]*Account
	kvstore map[string][]byte
	root    types.Hash
}

// NewPersistentState opens or creates a BoltDB database at the given path.
// The sync parameter controls whether writes are synchronous (fsynced to disk)
// or asynchronous (faster but less durable).
func NewPersistentState(path string, hasher types.Hasher, sync bool) (*PersistentState, error) {
	opts := &bbolt.Options{
		Timeout: 1 * time.Second,
		NoSync:  !sync,
	}
	db, err := bbolt.Open(path, 0600, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Create buckets if they don't exist
	if err := db.Update(func(tx *bbolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(accountsBucket); err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists(kvstoreBucket); err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists(metaBucket); err != nil {
			return err
		}
		return nil
	}); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create buckets: %w", err)
	}

	s := &PersistentState{
		db:      db,
		hasher:  hasher,
		cache:   make(map[types.Address]*Account),
		kvstore: make(map[string][]byte),
	}

	// Recover previous state root
	if err := db.View(func(tx *bbolt.Tx) error {
		root := tx.Bucket(metaBucket).Get(stateRootKey)
		if root != nil {
			copy(s.root[:], root)
		}
		return nil
	}); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to recover state root: %w", err)
	}

	// Load all accounts into cache
	if s.root != (types.Hash{}) {
		if err := s.loadAccounts(); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to load accounts: %w", err)
		}
		if err := s.loadKvstore(); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to load kvstore: %w", err)
		}
	}

	return s, nil
}

func (s *PersistentState) loadKvstore() error {
	return s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(kvstoreBucket)
		return b.ForEach(func(k, v []byte) error {
			s.kvstore[string(k)] = append([]byte{}, v...)
			return nil
		})
	})
}

func (s *PersistentState) loadAccounts() error {
	return s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(accountsBucket)
		return b.ForEach(func(k, v []byte) error {
			addr, err := types.AddressFromBytes(k)
			if err != nil {
				return fmt.Errorf("invalid account key %x: %w", k, err)
			}

			acc := &Account{}
			if err := acc.Decode(bytes.NewReader(v)); err != nil {
				return fmt.Errorf("corrupt account entry %x: %w", k, err)
			}

			s.cache[addr] = acc
			return nil
		})
	})
}

func (s *PersistentState) GetAccount(addr types.Address) (*Account, error) {
	acc, ok := s.cache[addr]
	if !ok {
		return nil, fmt.Errorf("account not found: %s", addr.String())
	}
	return acc, nil
}

func (s *PersistentState) SetAccount(addr types.Address, account *Account) error {
	s.cache[addr] = account
	return nil
}

func (s *PersistentState) DeleteAccount(addr types.Address) error {
	delete(s.cache, addr)
	return nil
}

// GetBytes retrieves a value from the kvstore.
func (s *PersistentState) GetBytes(key string) ([]byte, bool) {
	val, ok := s.kvstore[key]
	return val, ok
}

// SetBytes stores a value in the kvstore.
func (s *PersistentState) SetBytes(key string, value []byte) error {
	s.kvstore[key] = value
	return nil
}

// DeleteBytes removes a key from the kvstore.
func (s *PersistentState) DeleteBytes(key string) error {
	delete(s.kvstore, key)
	return nil
}

func (s *PersistentState) Commit() (types.Hash, error) {
	// Build SMT for state root (accounts + kvstore)
	hasher := s.hasher
	smt := NewSMT(hasher)

	// Sort account addresses for deterministic insertion
	addrs := make([]types.Address, 0, len(s.cache))
	for addr := range s.cache {
		addrs = append(addrs, addr)
	}
	sort.Slice(addrs, func(i, j int) bool {
		return bytes.Compare(addrs[i][:], addrs[j][:]) < 0
	})
	for _, addr := range addrs {
		acc := s.cache[addr]
		var buf bytes.Buffer
		if err := acc.Encode(&buf); err != nil {
			return types.Hash{}, err
		}
		if err := smt.Insert(addr.Bytes(), buf.Bytes()); err != nil {
			return types.Hash{}, err
		}
	}

	// Insert kvstore entries in sorted key order
	kvKeys := make([]string, 0, len(s.kvstore))
	for k := range s.kvstore {
		kvKeys = append(kvKeys, k)
	}
	sort.Strings(kvKeys)
	for _, k := range kvKeys {
		if err := smt.Insert([]byte(k), s.kvstore[k]); err != nil {
			return types.Hash{}, err
		}
	}

	s.root = smt.Root()

	// Persist all data in a single bbolt transaction
	if err := s.db.Update(func(tx *bbolt.Tx) error {
		ab := tx.Bucket(accountsBucket)
		kb := tx.Bucket(kvstoreBucket)
		mb := tx.Bucket(metaBucket)

		// Write all accounts
		for _, addr := range addrs {
			acc := s.cache[addr]
			var buf bytes.Buffer
			if err := acc.Encode(&buf); err != nil {
				return err
			}
			if err := ab.Put(addr.Bytes(), buf.Bytes()); err != nil {
				return err
			}
		}

		// Write all kvstore entries
		for _, k := range kvKeys {
			if err := kb.Put([]byte(k), s.kvstore[k]); err != nil {
				return err
			}
		}

		// Write state root
		if err := mb.Put(stateRootKey, s.root[:]); err != nil {
			return err
		}

		return nil
	}); err != nil {
		return types.Hash{}, err
	}

	return s.root, nil
}

func (s *PersistentState) GetStateRoot() types.Hash {
	return s.root
}

// ForEachAccount iterates over all cached accounts.
func (s *PersistentState) ForEachAccount(fn func(types.Address, *Account) error) error {
	for addr, acc := range s.cache {
		if err := fn(addr, acc); err != nil {
			return err
		}
	}
	return nil
}

// ForEachKV iterates over all cached kvstore entries.
func (s *PersistentState) ForEachKV(fn func(string, []byte) error) error {
	for k, v := range s.kvstore {
		if err := fn(k, v); err != nil {
			return err
		}
	}
	return nil
}

func (s *PersistentState) Close() error {
	return s.db.Close()
}

// DB returns the underlying BoltDB instance for direct bucket access.
// This allows other packages (e.g., consensus) to manage additional buckets.
func (s *PersistentState) DB() *bbolt.DB {
	return s.db
}
