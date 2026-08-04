package state

import (
	"bytes"
	"testing"

	"github.com/BryanOx/dsn/types"
)

func TestNewAccount(t *testing.T) {
	addr := types.Address([20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
	pubKey := [PublicKeySize]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	acc := NewAccount(addr, pubKey)

	if acc.Address != addr {
		t.Errorf("Address = %v, want %v", acc.Address, addr)
	}
	if acc.Nonce != 0 {
		t.Errorf("Nonce = %d, want 0", acc.Nonce)
	}
	if !acc.StorageRoot.IsZero() {
		t.Error("StorageRoot should be zero")
	}
	if !acc.CodeHash.IsZero() {
		t.Error("CodeHash should be zero")
	}
	if acc.Permissions != 0 {
		t.Errorf("Permissions = %d, want 0", acc.Permissions)
	}
	if !bytes.Equal(acc.PublicKey[:], pubKey[:]) {
		t.Error("PublicKey not stored correctly")
	}
}

func TestNewAccountPublicKey(t *testing.T) {
	addr := types.Address([20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
	pubKey := [PublicKeySize]byte{42, 42, 42, 42, 42, 42, 42, 42, 42, 42, 42, 42, 42, 42, 42, 42, 42, 42, 42, 42, 42, 42, 42, 42, 42, 42, 42, 42, 42, 42, 42, 42}

	acc := NewAccount(addr, pubKey)

	// Verify public key is stored correctly
	for i := 0; i < PublicKeySize; i++ {
		if acc.PublicKey[i] != pubKey[i] {
			t.Errorf("PublicKey[%d] = %d, want %d", i, acc.PublicKey[i], pubKey[i])
		}
	}
}

func TestAccountPublicKeyBytes(t *testing.T) {
	addr := types.Address([20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
	pubKey := [PublicKeySize]byte{9, 8, 7, 6, 5, 4, 3, 2, 1, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1, 2}

	acc := NewAccount(addr, pubKey)

	got := acc.PublicKey[:]
	want := pubKey[:]

	if !bytes.Equal(got, want) {
		t.Errorf("PublicKey.Bytes() = %v, want %v", got, want)
	}
}

func TestNewAccountZeroBalance(t *testing.T) {
	addr := types.Address([20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
	pubKey := [PublicKeySize]byte{}
	acc := NewAccount(addr, pubKey)

	if !acc.Balance.IsZero() {
		t.Error("Balance should be zero")
	}
}

func TestAddBalance(t *testing.T) {
	addr := types.Address([20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
	pubKey := [PublicKeySize]byte{}
	acc := NewAccount(addr, pubKey)

	err := acc.AddBalance(types.NewAmount(100))
	if err != nil {
		t.Fatalf("AddBalance failed: %v", err)
	}

	if acc.Balance.Cmp(types.NewAmount(100)) != 0 {
		t.Errorf("Balance = %s, want 100", acc.Balance)
	}
}

func TestAddBalanceOverflow(t *testing.T) {
	// Note: Amount type supports 128-bit, which is far beyond uint64 range.
	// Adding max uint64 repeatedly will not overflow. This test verifies
	// the function handles large additions gracefully.
	addr := types.Address([20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
	pubKey := [PublicKeySize]byte{}
	acc := NewAccount(addr, pubKey)

	// Add multiple max uint64 values - still won't overflow 128-bit
	for i := 0; i < 100; i++ {
		err := acc.AddBalance(types.NewAmount(1<<63 - 1))
		if err != nil {
			t.Fatalf("AddBalance failed at iteration %d: %v", i, err)
		}
	}

	// Balance should be accumulated
	if acc.Balance.IsZero() {
		t.Error("Balance should not be zero after additions")
	}
}

func TestSubBalance(t *testing.T) {
	addr := types.Address([20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
	pubKey := [PublicKeySize]byte{}
	acc := NewAccount(addr, pubKey)

	acc.AddBalance(types.NewAmount(100))

	err := acc.SubBalance(types.NewAmount(30))
	if err != nil {
		t.Fatalf("SubBalance failed: %v", err)
	}

	if acc.Balance.Cmp(types.NewAmount(70)) != 0 {
		t.Errorf("Balance = %s, want 70", acc.Balance)
	}
}

func TestSubBalanceUnderflow(t *testing.T) {
	addr := types.Address([20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
	pubKey := [PublicKeySize]byte{}
	acc := NewAccount(addr, pubKey)

	err := acc.SubBalance(types.NewAmount(1))
	if err == nil {
		t.Error("SubBalance should fail on underflow")
	}
}

func TestIncrementNonce(t *testing.T) {
	addr := types.Address([20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
	pubKey := [PublicKeySize]byte{}
	acc := NewAccount(addr, pubKey)

	if acc.Nonce != 0 {
		t.Errorf("Initial Nonce = %d, want 0", acc.Nonce)
	}

	acc.IncrementNonce()
	if acc.Nonce != 1 {
		t.Errorf("After IncrementNonce, Nonce = %d, want 1", acc.Nonce)
	}

	acc.IncrementNonce()
	if acc.Nonce != 2 {
		t.Errorf("After second IncrementNonce, Nonce = %d, want 2", acc.Nonce)
	}
}
