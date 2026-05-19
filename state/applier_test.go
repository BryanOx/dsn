package state

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/dsn/dsn/types"
)

func TestApplyTransaction_Valid(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s := NewInMemoryState(hasher)

	// Generate key pair
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	// Derive address from public key (first 20 bytes)
	addr, err := types.AddressFromBytes(pub[:20])
	if err != nil {
		t.Fatal(err)
	}

	// Create sender account with public key
	var pubKey [PublicKeySize]byte
	copy(pubKey[:], pub)
	acc := NewAccount(addr, pubKey)
	acc.AddBalance(types.NewAmount(10000))
	s.SetAccount(addr, acc)

	// Build a transfer-like transaction
	tx := types.NewTransaction(1, 0, addr, 1, []byte("data"), nil, 1000, 50000, uint64(time.Now().Unix()))

	// Compute IntentID
	intentID, _ := tx.ComputeIntentID(hasher)
	tx.IntentID = intentID

	// Sign
	sig := ed25519.Sign(priv, intentID[:])
	tx.Signature = sig

	// Apply
	_, err = ApplyTransaction(s, tx, hasher, 1)
	if err != nil {
		t.Fatal(err)
	}

	// Verify nonce incremented
	sender, _ := s.GetAccount(addr)
	if sender.Nonce != 1 {
		t.Errorf("nonce = %d, want 1", sender.Nonce)
	}
}

func TestApplyTransaction_InvalidSignature(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s := NewInMemoryState(hasher)

	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	addr, _ := types.AddressFromBytes(pub[:20])

	var pubKey [PublicKeySize]byte
	copy(pubKey[:], pub)
	s.SetAccount(addr, NewAccount(addr, pubKey))

	tx := types.NewTransaction(1, 0, addr, 1, []byte("data"), nil, 1000, 50000, uint64(time.Now().Unix()))
	intentID, _ := tx.ComputeIntentID(hasher)
	tx.IntentID = intentID
	tx.Signature = []byte("fake signature") // invalid

	_, err := ApplyTransaction(s, tx, hasher, 1)
	if err == nil {
		t.Fatal("expected invalid signature error")
	}
}

func TestApplyTransaction_WrongNonce(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s := NewInMemoryState(hasher)

	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	addr, _ := types.AddressFromBytes(pub[:20])

	var pubKey [PublicKeySize]byte
	copy(pubKey[:], pub)
	s.SetAccount(addr, NewAccount(addr, pubKey))

	// Nonce should start at 0, so nonce=5 is wrong
	tx := types.NewTransaction(1, 0, addr, 5, []byte("data"), nil, 1000, 50000, uint64(time.Now().Unix()))
	intentID, _ := tx.ComputeIntentID(hasher)
	tx.IntentID = intentID
	tx.Signature = ed25519.Sign(priv, intentID[:])

	_, err := ApplyTransaction(s, tx, hasher, 1)
	if err == nil {
		t.Fatal("expected nonce mismatch error")
	}
}

func TestApplyTransaction_InsufficientFunds(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s := NewInMemoryState(hasher)

	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	addr, _ := types.AddressFromBytes(pub[:20])

	var pubKey [PublicKeySize]byte
	copy(pubKey[:], pub)
	acc := NewAccount(addr, pubKey)
	acc.AddBalance(types.NewAmount(10)) // very low balance
	s.SetAccount(addr, acc)

	tx := types.NewTransaction(1, 0, addr, 1, []byte("data"), nil, 1000, 50000, uint64(time.Now().Unix()))
	intentID, _ := tx.ComputeIntentID(hasher)
	tx.IntentID = intentID
	tx.Signature = ed25519.Sign(priv, intentID[:])

	_, err := ApplyTransaction(s, tx, hasher, 1)
	if err == nil {
		t.Fatal("expected insufficient funds error")
	}
}

func TestApplyTransaction_SenderNotFound(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s := NewInMemoryState(hasher)

	addr := types.Address([20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})

	tx := types.NewTransaction(1, 0, addr, 1, nil, nil, 1000, 50000, uint64(time.Now().Unix()))

	_, err := ApplyTransaction(s, tx, hasher, 1)
	if err == nil {
		t.Fatal("expected sender not found error")
	}
}