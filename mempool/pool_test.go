package mempool

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/dsn/dsn/state"
	"github.com/dsn/dsn/types"
)

// Helper to create a test account with valid Ed25519 key
func createTestAccountWithKey(s *state.InMemoryState) (types.Address, [32]byte, ed25519.PrivateKey) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}

	addr, err := types.AddressFromBytes(pub[:20])
	if err != nil {
		panic(err)
	}

	var pubKey [32]byte
	copy(pubKey[:], pub)

	acc := state.NewAccount(addr, pubKey)
	acc.AddBalance(types.NewAmount(100000))
	s.SetAccount(addr, acc)

	return addr, pubKey, priv
}

// Helper to create a valid signed tx for testing
func makeTestTx(nonce uint64, maxFee uint64, ts uint64, addr types.Address, priv ed25519.PrivateKey, intentID types.Hash) *types.Transaction {
	tx := &types.Transaction{
		Version:   1,
		ChainID:   0,
		Sender:    addr,
		Nonce:     nonce,
		Payload:   []byte("data"),
		MaxFee:    maxFee,
		GasLimit:  50000,
		Timestamp: ts,
		IntentID:  intentID,
	}

	// Sign the IntentID
	tx.Signature = ed25519.Sign(priv, tx.IntentID[:])
	return tx
}

func TestTxIndex_InsertAndOrder(t *testing.T) {
	idx := newTxIndex()

	// Index tests don't need valid signatures - just need distinct txs
	now := uint64(time.Now().Unix())
	dummyAddr := types.Address([20]byte{1})

	tx1 := &types.Transaction{
		Version: 1, ChainID: 0, Sender: dummyAddr, Nonce: 1,
		MaxFee: 100, Timestamp: now, IntentID: types.Hash{1}, Signature: []byte("sig"),
	}
	tx2 := &types.Transaction{
		Version: 1, ChainID: 0, Sender: dummyAddr, Nonce: 2,
		MaxFee: 200, Timestamp: now, IntentID: types.Hash{2}, Signature: []byte("sig"),
	}
	tx3 := &types.Transaction{
		Version: 1, ChainID: 0, Sender: dummyAddr, Nonce: 3,
		MaxFee: 100, Timestamp: now - 10, IntentID: types.Hash{3}, Signature: []byte("sig"),
	}

	idx.Insert(tx1)
	idx.Insert(tx2)
	idx.Insert(tx3)

	sorted := idx.Sorted()
	if len(sorted) != 3 {
		t.Fatalf("expected 3, got %d", len(sorted))
	}

	// Highest fee first
	if sorted[0].MaxFee != 200 {
		t.Errorf("first should be fee=200, got %d", sorted[0].MaxFee)
	}

	// Same fee → older timestamp first
	if sorted[1].MaxFee == 100 {
		// Both have fee=100, older timestamp should be first
		if sorted[1].Timestamp > sorted[2].Timestamp {
			t.Error("same fee should order by timestamp ASC")
		}
	}
}

func TestTxIndex_Remove(t *testing.T) {
	idx := newTxIndex()
	dummyAddr := types.Address([20]byte{1})
	tx := &types.Transaction{
		Version: 1, ChainID: 0, Sender: dummyAddr, Nonce: 1,
		MaxFee: 100, Timestamp: uint64(time.Now().Unix()), IntentID: types.Hash{1}, Signature: []byte("sig"),
	}
	idx.Insert(tx)
	idx.Remove(tx.IntentID)

	if len(idx.Sorted()) != 0 {
		t.Error("should be empty after remove")
	}
}

func TestTxIndex_Empty(t *testing.T) {
	idx := newTxIndex()
	if len(idx.Sorted()) != 0 {
		t.Error("empty index should return empty slice")
	}
}

func TestNewMempool(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s := state.NewInMemoryState(hasher)
	mp := New(1000, 300*time.Second, s)
	if mp == nil {
		t.Fatal("New returned nil")
	}
	if mp.Count() != 0 {
		t.Error("new mempool should have 0 transactions")
	}
}

func TestSubmit_Valid(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s := state.NewInMemoryState(hasher)

	// Create sender account with valid key
	addr, _, priv := createTestAccountWithKey(s)

	mp := New(1000, 300*time.Second, s)

	tx := makeTestTx(1, 100, uint64(time.Now().Unix()), addr, priv, types.Hash{1, 2, 3})

	err := mp.Submit(tx)
	if err != nil {
		t.Fatal(err)
	}
	if mp.Count() != 1 {
		t.Errorf("count = %d, want 1", mp.Count())
	}
}

func TestSubmit_Duplicate(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s := state.NewInMemoryState(hasher)
	addr, _, priv := createTestAccountWithKey(s)

	mp := New(1000, 300*time.Second, s)

	tx := makeTestTx(1, 100, uint64(time.Now().Unix()), addr, priv, types.Hash{1})

	if err := mp.Submit(tx); err != nil {
		t.Fatal(err)
	}
	if err := mp.Submit(tx); err == nil {
		t.Fatal("expected ErrAlreadyInPool")
	}
}

func TestSubmit_WrongNonce(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s := state.NewInMemoryState(hasher)
	addr, _, priv := createTestAccountWithKey(s)

	mp := New(1000, 300*time.Second, s)

	// Nonce must be account.Nonce + 1 = 1, so nonce=5 is wrong
	tx := makeTestTx(5, 100, uint64(time.Now().Unix()), addr, priv, types.Hash{1})

	err := mp.Submit(tx)
	if err == nil {
		t.Fatal("expected nonce error")
	}
}

func TestSubmit_InsufficientBalance(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s := state.NewInMemoryState(hasher)

	// Create account with insufficient balance
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	addr, err := types.AddressFromBytes(pub[:20])
	if err != nil {
		t.Fatal(err)
	}
	var pubKey [32]byte
	copy(pubKey[:], pub)
	acc := state.NewAccount(addr, pubKey)
	acc.AddBalance(types.NewAmount(10)) // only 10, fee is 100
	s.SetAccount(addr, acc)

	mp := New(1000, 300*time.Second, s)

	tx := makeTestTx(1, 100, uint64(time.Now().Unix()), addr, priv, types.Hash{1})

	err = mp.Submit(tx)
	if err == nil {
		t.Fatal("expected insufficient balance error")
	}
}

func TestPendingTxs_Ordering(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s := state.NewInMemoryState(hasher)
	addr, _, priv := createTestAccountWithKey(s)

	mp := New(1000, 300*time.Second, s)

	now := uint64(time.Now().Unix())
	tx1 := makeTestTx(1, 100, now, addr, priv, types.Hash{1})
	tx2 := makeTestTx(2, 200, now, addr, priv, types.Hash{2})
	tx3 := makeTestTx(3, 150, now, addr, priv, types.Hash{3})

	// Submit in correct nonce order (mempool validates nonces)
	mp.Submit(tx1)
	mp.Submit(tx2)
	mp.Submit(tx3)

	pending := mp.PendingTxs()
	if len(pending) != 3 {
		t.Fatalf("expected 3 pending, got %d", len(pending))
	}

	// Should be ordered by fee DESC: 200, 150, 100
	if pending[0].MaxFee != 200 {
		t.Errorf("first should be fee=200, got %d", pending[0].MaxFee)
	}
	if pending[1].MaxFee != 150 {
		t.Errorf("second should be fee=150, got %d", pending[1].MaxFee)
	}
	if pending[2].MaxFee != 100 {
		t.Errorf("third should be fee=100, got %d", pending[2].MaxFee)
	}
}

func TestRemove(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s := state.NewInMemoryState(hasher)
	addr, _, priv := createTestAccountWithKey(s)

	mp := New(1000, 300*time.Second, s)

	tx := makeTestTx(1, 100, uint64(time.Now().Unix()), addr, priv, types.Hash{42})
	mp.Submit(tx)
	mp.Remove(types.Hash{42})

	if mp.Count() != 0 {
		t.Error("should be empty after remove")
	}
}

func TestMempoolFull(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s := state.NewInMemoryState(hasher)
	addr, _, priv := createTestAccountWithKey(s)

	mp := New(3, 300*time.Second, s) // max 3

	for i := 1; i <= 3; i++ {
		tx := makeTestTx(uint64(i), 100, uint64(time.Now().Unix()), addr, priv, types.Hash{byte(i)})
		if err := mp.Submit(tx); err != nil {
			t.Fatalf("submit %d failed: %v", i, err)
		}
	}

	// 4th should fail
	tx4 := makeTestTx(4, 100, uint64(time.Now().Unix()), addr, priv, types.Hash{4})
	if err := mp.Submit(tx4); err == nil {
		t.Fatal("expected ErrMempoolFull")
	}
}
