package state

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/BryanOx/dsn/types"
)

func setupTempDB(t *testing.T) (*PersistentState, string) {
	t.Helper()
	dir, err := os.MkdirTemp("", "dsn-test-*")
	if err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dir, "dsn.db")

	hasher := types.SHA256Hasher{}
	s, err := NewPersistentState(dbPath, hasher, false)
	if err != nil {
		os.RemoveAll(dir)
		t.Fatal(err)
	}

	return s, dir
}

func teardownTempDB(s *PersistentState, dir string) {
	s.Close()
	os.RemoveAll(dir)
}

func TestPersistentState_New(t *testing.T) {
	s, dir := setupTempDB(t)
	defer teardownTempDB(s, dir)

	if s == nil {
		t.Fatal("PersistentState is nil")
	}
}

func TestPersistentState_SetAndGet(t *testing.T) {
	s, dir := setupTempDB(t)
	defer teardownTempDB(s, dir)

	addr := types.Address([20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
	acc := NewAccount(addr, [PublicKeySize]byte{})
	acc.AddBalance(types.NewAmount(5000))

	err := s.SetAccount(addr, acc)
	if err != nil {
		t.Fatal(err)
	}

	got, err := s.GetAccount(addr)
	if err != nil {
		t.Fatal(err)
	}
	if got.Balance.Cmp(types.NewAmount(5000)) != 0 {
		t.Errorf("balance = %s, want 5000", got.Balance)
	}
}

func TestPersistentState_GetNonExistent(t *testing.T) {
	s, dir := setupTempDB(t)
	defer teardownTempDB(s, dir)

	addr := types.Address([20]byte{42})
	_, err := s.GetAccount(addr)
	if err == nil {
		t.Error("expected error for non-existent account")
	}
}

func TestPersistentState_Delete(t *testing.T) {
	s, dir := setupTempDB(t)
	defer teardownTempDB(s, dir)

	addr := types.Address([20]byte{1})
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

func TestPersistentState_CommitAndStateRoot(t *testing.T) {
	s, dir := setupTempDB(t)
	defer teardownTempDB(s, dir)

	addr := types.Address([20]byte{1})
	s.SetAccount(addr, NewAccount(addr, [PublicKeySize]byte{}))

	root, err := s.Commit()
	if err != nil {
		t.Fatal(err)
	}

	var zeroHash types.Hash
	if root == zeroHash {
		t.Error("state root should not be zero")
	}

	got := s.GetStateRoot()
	if got != root {
		t.Error("GetStateRoot should return last committed root")
	}
}

func TestPersistentState_RestartRecovery(t *testing.T) {
	dir, err := os.MkdirTemp("", "dsn-restart-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	dbPath := filepath.Join(dir, "dsn.db")

	hasher := types.SHA256Hasher{}

	// First session
	s1, _ := NewPersistentState(dbPath, hasher, false)
	addr := types.Address([20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
	acc := NewAccount(addr, [PublicKeySize]byte{})
	acc.AddBalance(types.NewAmount(10000))
	s1.SetAccount(addr, acc)
	root1, _ := s1.Commit()
	s1.Close()

	// Second session (simulate restart)
	s2, err := NewPersistentState(dbPath, hasher, false)
	if err != nil {
		t.Fatal("failed to restart from existing DB")
	}
	defer s2.Close()

	// Account should exist
	got, err := s2.GetAccount(addr)
	if err != nil {
		t.Fatal("account lost after restart:", err)
	}
	if got.Balance.Cmp(types.NewAmount(10000)) != 0 {
		t.Errorf("balance = %s, want 10000", got.Balance)
	}

	// State root should match
	root2 := s2.GetStateRoot()
	if root2 != root1 {
		t.Error("state root changed after restart")
	}
}

func TestPersistentState_Close(t *testing.T) {
	s, dir := setupTempDB(t)
	defer os.RemoveAll(dir)

	err := s.Close()
	if err != nil {
		t.Fatal(err)
	}
}

// TestFSyncDeterminism verifies that FSync=true produces the same state roots
// as FSync=false. This ensures the sync mode doesn't affect determinism.
func TestFSyncDeterminism(t *testing.T) {
	// Create two temporary directories
	dir1, err := os.MkdirTemp("", "dsn-fsync-true-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir1)

	dir2, err := os.MkdirTemp("", "dsn-fsync-false-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir2)

	hasher := types.SHA256Hasher{}

	// Create two PersistentStates with different FSync settings
	ps1, err := NewPersistentState(filepath.Join(dir1, "dsn.db"), hasher, true) // FSync=true
	if err != nil {
		t.Fatal(err)
	}
	defer ps1.Close()

	ps2, err := NewPersistentState(filepath.Join(dir2, "dsn.db"), hasher, false) // FSync=false
	if err != nil {
		t.Fatal(err)
	}
	defer ps2.Close()

	// Add identical accounts to both
	addr1 := types.Address([20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
	addr2 := types.Address([20]byte{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21})

	acc1 := NewAccount(addr1, [PublicKeySize]byte{})
	acc1.AddBalance(types.NewAmount(1000))
	ps1.SetAccount(addr1, acc1)
	ps2.SetAccount(addr1, acc1)

	acc2 := NewAccount(addr2, [PublicKeySize]byte{})
	acc2.AddBalance(types.NewAmount(2000))
	ps1.SetAccount(addr2, acc2)
	ps2.SetAccount(addr2, acc2)

	// Add identical kvstore entries
	ps1.SetBytes("key1", []byte("value1"))
	ps2.SetBytes("key1", []byte("value1"))
	ps1.SetBytes("key2", []byte("value2"))
	ps2.SetBytes("key2", []byte("value2"))

	// Commit both
	root1, err := ps1.Commit()
	if err != nil {
		t.Fatal(err)
	}

	root2, err := ps2.Commit()
	if err != nil {
		t.Fatal(err)
	}

	// State roots must be identical regardless of FSync setting
	if root1 != root2 {
		t.Errorf("FSync=true state root = %v, FSync=false state root = %v - they must be equal for determinism", root1, root2)
	}
}
