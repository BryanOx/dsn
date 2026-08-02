package state

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/dsn/dsn/types"
)

func TestInMemoryState_New(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s := NewInMemoryState(hasher)
	if s == nil {
		t.Fatal("NewInMemoryState returned nil")
	}
}

func TestInMemoryState_GetSetAccount(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s := NewInMemoryState(hasher)

	addr := types.Address([20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
	var pubKey [PublicKeySize]byte
	pubKey[0] = 42

	acc := NewAccount(addr, pubKey)
	acc.AddBalance(types.NewAmount(1000))

	err := s.SetAccount(addr, acc)
	if err != nil {
		t.Fatal(err)
	}

	got, err := s.GetAccount(addr)
	if err != nil {
		t.Fatal(err)
	}
	if got.Balance.Cmp(types.NewAmount(1000)) != 0 {
		t.Errorf("balance = %s, want 1000", got.Balance)
	}
}

func TestInMemoryState_GetNonExistentAccount(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s := NewInMemoryState(hasher)

	addr := types.Address([20]byte{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1})
	_, err := s.GetAccount(addr)
	if err == nil {
		t.Error("expected error for non-existent account")
	}
}

func TestInMemoryState_DeleteAccount(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s := NewInMemoryState(hasher)

	addr := types.Address([20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
	s.SetAccount(addr, NewAccount(addr, [PublicKeySize]byte{}))

	err := s.DeleteAccount(addr)
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.GetAccount(addr)
	if err == nil {
		t.Error("account should be deleted")
	}
}

func TestInMemoryState_Commit(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s := NewInMemoryState(hasher)

	addr := types.Address([20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
	s.SetAccount(addr, NewAccount(addr, [PublicKeySize]byte{}))

	root, err := s.Commit()
	if err != nil {
		t.Fatal(err)
	}

	var zeroHash types.Hash
	if root == zeroHash {
		t.Error("commit root should not be zero after SetAccount")
	}
}

func TestInMemoryState_GetStateRoot(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s := NewInMemoryState(hasher)

	root0 := s.GetStateRoot()

	addr := types.Address([20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
	s.SetAccount(addr, NewAccount(addr, [PublicKeySize]byte{}))
	s.Commit()

	root1 := s.GetStateRoot()

	if root0 == root1 {
		t.Error("state root should change after commit with new account")
	}
}

func TestInMemoryState_CommitDeterminism(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s1 := NewInMemoryState(hasher)
	s2 := NewInMemoryState(hasher)

	addr1 := types.Address([20]byte{1})
	addr2 := types.Address([20]byte{2})

	s1.SetAccount(addr1, NewAccount(addr1, [PublicKeySize]byte{}))
	s1.SetAccount(addr2, NewAccount(addr2, [PublicKeySize]byte{}))
	root1, _ := s1.Commit()

	s2.SetAccount(addr2, NewAccount(addr2, [PublicKeySize]byte{}))
	s2.SetAccount(addr1, NewAccount(addr1, [PublicKeySize]byte{}))
	root2, _ := s2.Commit()

	if root1 != root2 {
		t.Error("same accounts in different order should produce same root")
	}
}

func TestInMemoryState_ConcurrentReads(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s := NewInMemoryState(hasher)

	addr := types.Address([20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
	s.SetAccount(addr, NewAccount(addr, [PublicKeySize]byte{}))

	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			s.GetAccount(addr)
			s.GetStateRoot()
			done <- true
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}

// TestSnapshotRevertToSnapshot tests the snapshot and rollback functionality
func TestSnapshotRevertToSnapshot(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s := NewInMemoryState(hasher)

	addr := types.Address([20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
	acc := NewAccount(addr, [PublicKeySize]byte{})
	acc.AddBalance(types.NewAmount(100))

	// Set initial account
	err := s.SetAccount(addr, acc)
	if err != nil {
		t.Fatal(err)
	}

	// Take snapshot
	snapID := s.Snapshot()

	// Modify account
	acc.AddBalance(types.NewAmount(50))
	err = s.SetAccount(addr, acc)
	if err != nil {
		t.Fatal(err)
	}

	// Verify balance changed
	got, _ := s.GetAccount(addr)
	if got.Balance.Cmp(types.NewAmount(150)) != 0 {
		t.Fatalf("balance = %s, want 150", got.Balance)
	}

	// Revert to snapshot
	err = s.RevertToSnapshot(snapID)
	if err != nil {
		t.Fatal(err)
	}

	// Verify balance reverted
	got, _ = s.GetAccount(addr)
	if got.Balance.Cmp(types.NewAmount(100)) != 0 {
		t.Fatalf("after revert, balance = %s, want 100", got.Balance)
	}
}

// TestSnapshotMultipleSequential tests multiple sequential snapshots
func TestSnapshotMultipleSequential(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s := NewInMemoryState(hasher)

	addr := types.Address([20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})

	// Take first snapshot with initial state (empty accounts)
	snap0 := s.Snapshot()

	// Set account to balance 100
	acc := NewAccount(addr, [PublicKeySize]byte{})
	acc.AddBalance(types.NewAmount(100))
	s.SetAccount(addr, acc)

	// Take second snapshot (accounts at balance 100)
	snap1 := s.Snapshot()

	// Modify to balance 200
	acc.AddBalance(types.NewAmount(100))
	s.SetAccount(addr, acc)

	// Take third snapshot (accounts at balance 200)
	_ = s.Snapshot()

	// Modify to balance 300
	acc.AddBalance(types.NewAmount(100))
	s.SetAccount(addr, acc)

	// Revert to second snapshot (snap1) - should restore to balance 100
	err := s.RevertToSnapshot(snap1)
	if err != nil {
		t.Fatal(err)
	}

	// Should be at balance 100 (what snap1 captured)
	got, _ := s.GetAccount(addr)
	if got.Balance.Cmp(types.NewAmount(100)) != 0 {
		t.Fatalf("after revert to snap1, balance = %s, want 100", got.Balance)
	}

	// Revert to first snapshot (snap0) - should restore to empty state
	err = s.RevertToSnapshot(snap0)
	if err != nil {
		t.Fatal(err)
	}

	// Account should not exist (reverted to empty state)
	_, err = s.GetAccount(addr)
	if err == nil {
		t.Error("account should not exist after revert to snap0")
	}

	// Now test going forward again - create new snapshots after revert
	// Take new snapshot at empty state
	_ = s.Snapshot()

	// Set account to balance 50
	acc = NewAccount(addr, [PublicKeySize]byte{})
	acc.AddBalance(types.NewAmount(50))
	s.SetAccount(addr, acc)

	// Take snapshot at balance 50
	snap1 = s.Snapshot()

	// Modify to balance 150
	acc.AddBalance(types.NewAmount(100))
	s.SetAccount(addr, acc)

	// Revert to snap1 - should restore to balance 50
	err = s.RevertToSnapshot(snap1)
	if err != nil {
		t.Fatal(err)
	}

	got, _ = s.GetAccount(addr)
	if got.Balance.Cmp(types.NewAmount(50)) != 0 {
		t.Fatalf("after revert to snap1, balance = %s, want 50", got.Balance)
	}
}

// TestSnapshotRevertInvalid tests invalid snapshot IDs
func TestSnapshotRevertInvalid(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s := NewInMemoryState(hasher)

	// Revert to negative ID
	err := s.RevertToSnapshot(-1)
	if err != ErrInvalidSnapshot {
		t.Errorf("expected ErrInvalidSnapshot, got %v", err)
	}

	// Revert to non-existent ID
	err = s.RevertToSnapshot(999)
	if err != ErrInvalidSnapshot {
		t.Errorf("expected ErrInvalidSnapshot, got %v", err)
	}

	// Take a snapshot, then try to revert beyond it
	s.Snapshot()
	err = s.RevertToSnapshot(5)
	if err != ErrInvalidSnapshot {
		t.Errorf("expected ErrInvalidSnapshot, got %v", err)
	}
}

// TestContractStorageNamespace tests that contract storage keys are properly namespaced
func TestContractStorageNamespace(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s := NewInMemoryState(hasher)

	// Two different contracts
	contractIDA := types.Hash([32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32})
	contractIDB := types.Hash([32]byte{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33})

	// Same logical key for both contracts
	key := []byte("balance")

	// Write to contract A
	err := s.SetContractStorage(contractIDA, key, []byte{0xAA})
	if err != nil {
		t.Fatal(err)
	}

	// Write to contract B
	err = s.SetContractStorage(contractIDB, key, []byte{0xBB})
	if err != nil {
		t.Fatal(err)
	}

	// Read back - should be independent
	valA, err := s.GetContractStorage(contractIDA, key)
	if err != nil {
		t.Fatal(err)
	}
	if valA[0] != 0xAA {
		t.Errorf("contract A storage = 0x%x, want 0xAA", valA[0])
	}

	valB, err := s.GetContractStorage(contractIDB, key)
	if err != nil {
		t.Fatal(err)
	}
	if valB[0] != 0xBB {
		t.Errorf("contract B storage = 0x%x, want 0xBB", valB[0])
	}
}

// TestContractStorageKeyHashing tests that storage keys are SHA-256 hashed
func TestContractStorageKeyHashing(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s := NewInMemoryState(hasher)

	contractID := types.Hash([32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32})

	// Long key that would be problematic without hashing
	longKey := make([]byte, 1024)
	for i := range longKey {
		longKey[i] = byte(i % 256)
	}

	// Write with long key
	err := s.SetContractStorage(contractID, longKey, []byte{0x01})
	if err != nil {
		t.Fatal(err)
	}

	// Read back
	val, err := s.GetContractStorage(contractID, longKey)
	if err != nil {
		t.Fatal(err)
	}
	if val[0] != 0x01 {
		t.Errorf("storage = 0x%x, want 0x01", val[0])
	}

	// Verify the key is hashed by checking the internal kvstore
	// The key should be: "storage:<contractID hex>:<sha256(longKey) hex>"
	expectedKeyHash := sha256.Sum256(longKey)
	expectedStorageKey := "storage:" + fmt.Sprintf("%x", contractID[:]) + ":" + fmt.Sprintf("%x", expectedKeyHash[:])

	s.mu.RLock()
	_, ok := s.kvstore[expectedStorageKey]
	s.mu.RUnlock()

	if !ok {
		t.Error("storage key not found in kvstore - key may not be properly hashed")
	}
}

// TestSnapshotContractStorageRollback tests that contract storage is rolled back correctly
func TestSnapshotContractStorageRollback(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s := NewInMemoryState(hasher)

	contractID := types.Hash([32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32})
	key := []byte("balance")

	// Take snapshot with empty storage
	snapID := s.Snapshot()

	// Write storage
	err := s.SetContractStorage(contractID, key, []byte{0xFF})
	if err != nil {
		t.Fatal(err)
	}

	// Verify storage exists
	val, _ := s.GetContractStorage(contractID, key)
	if val == nil || val[0] != 0xFF {
		t.Error("storage should exist after write")
	}

	// Revert to snapshot
	err = s.RevertToSnapshot(snapID)
	if err != nil {
		t.Fatal(err)
	}

	// Verify storage is gone
	_, err = s.GetContractStorage(contractID, key)
	if err == nil {
		t.Error("storage should not exist after revert")
	}
}
