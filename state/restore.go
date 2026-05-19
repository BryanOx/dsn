package state

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"sort"

	"github.com/dsn/dsn/types"
	"go.etcd.io/bbolt"
)

// RestoreFromSnapshot performs full deterministic state restoration from snapshot bytes.
// This is the core of fast sync and crash recovery.
func RestoreFromSnapshot(ps *PersistentState, snapData []byte, expectedHash types.Hash) error {
	// 1. Verify snapshot hash
	hash := sha256.Sum256(snapData)
	if expectedHash != (types.Hash{}) {
		if !bytes.Equal(hash[:], expectedHash[:]) {
			return fmt.Errorf("restore: snapshot hash mismatch: got %x, expected %x", hash[:], expectedHash[:])
		}
	}

	// 2. Deserialize snapshot
	snap, err := DeserializeSnapshot(snapData)
	if err != nil {
		return fmt.Errorf("restore: deserialize snapshot: %w", err)
	}

	// 3. Verify state root is non-zero (unless genesis snapshot)
	if snap.StateRoot == (types.Hash{}) {
		return fmt.Errorf("restore: zero state root in snapshot (genesis only)")
	}

	// 4. Clear existing state
	ps.cache = make(map[types.Address]*Account)
	ps.kvstore = make(map[string][]byte)

	// 5. Load accounts from snapshot
	for _, entry := range snap.Accounts {
		acc := &Account{}
		if err := acc.Decode(bytes.NewReader(entry.Data)); err != nil {
			return fmt.Errorf("restore: decode account %s: %w", entry.Address.String(), err)
		}
		ps.cache[entry.Address] = acc
	}

	// 6. Load kvstore from snapshot
	for _, entry := range snap.KVStore {
		ps.kvstore[entry.Key] = entry.Value
	}

	// 7. Verify state root by rebuilding SMT
	smt := NewSMT(ps.hasher)

	// Insert accounts in sorted address order (deterministic)
	addrs := make([]types.Address, 0, len(ps.cache))
	for addr := range ps.cache {
		addrs = append(addrs, addr)
	}
	sort.Slice(addrs, func(i, j int) bool {
		return bytes.Compare(addrs[i][:], addrs[j][:]) < 0
	})
	for _, addr := range addrs {
		acc := ps.cache[addr]
		var buf bytes.Buffer
		if err := acc.Encode(&buf); err != nil {
			return fmt.Errorf("restore: encode account: %w", err)
		}
		if err := smt.Insert(addr.Bytes(), buf.Bytes()); err != nil {
			return fmt.Errorf("restore: insert account into SMT: %w", err)
		}
	}

	// Insert kvstore entries in sorted key order (deterministic)
	kvKeys := make([]string, 0, len(ps.kvstore))
	for k := range ps.kvstore {
		kvKeys = append(kvKeys, k)
	}
	sort.Strings(kvKeys)
	for _, k := range kvKeys {
		if err := smt.Insert([]byte(k), ps.kvstore[k]); err != nil {
			return fmt.Errorf("restore: insert kvstore into SMT: %w", err)
		}
	}

	// Verify computed root matches snapshot state root
	root := smt.Root()
	if root != snap.StateRoot {
		return fmt.Errorf("restore: state root mismatch: got %x, expected %x", root[:], snap.StateRoot[:])
	}

	// 8. Persist atomically to BoltDB
	if err := ps.db.Update(func(tx *bbolt.Tx) error {
		// Clear and recreate accounts bucket
		if err := tx.DeleteBucket(accountsBucket); err != nil {
			return fmt.Errorf("restore: delete accounts bucket: %w", err)
		}
		ab, err := tx.CreateBucket(accountsBucket)
		if err != nil {
			return fmt.Errorf("restore: create accounts bucket: %w", err)
		}

		// Write all accounts
		for _, addr := range addrs {
			acc := ps.cache[addr]
			var buf bytes.Buffer
			if err := acc.Encode(&buf); err != nil {
				return fmt.Errorf("restore: encode account: %w", err)
			}
			if err := ab.Put(addr.Bytes(), buf.Bytes()); err != nil {
				return fmt.Errorf("restore: write account: %w", err)
			}
		}

		// Clear and recreate kvstore bucket
		if err := tx.DeleteBucket(kvstoreBucket); err != nil {
			return fmt.Errorf("restore: delete kvstore bucket: %w", err)
		}
		kb, err := tx.CreateBucket(kvstoreBucket)
		if err != nil {
			return fmt.Errorf("restore: create kvstore bucket: %w", err)
		}

		// Write all kvstore entries
		for _, k := range kvKeys {
			if err := kb.Put([]byte(k), ps.kvstore[k]); err != nil {
				return fmt.Errorf("restore: write kvstore: %w", err)
			}
		}

		// Write state root to meta bucket
		mb := tx.Bucket(metaBucket)
		if err := mb.Put(stateRootKey, snap.StateRoot[:]); err != nil {
			return fmt.Errorf("restore: write state root: %w", err)
		}

		return nil
	}); err != nil {
		return fmt.Errorf("restore: persist to bolt: %w", err)
	}

	// 9. Set in-memory root
	ps.root = snap.StateRoot

	return nil
}