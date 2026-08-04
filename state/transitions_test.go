package state

import (
	"testing"

	"github.com/BryanOx/dsn/types"
)

func TestTransfer_Success(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s := NewInMemoryState(hasher)

	aliceAddr := types.Address([20]byte{1})
	bobAddr := types.Address([20]byte{2})
	var pubKey [PublicKeySize]byte

	alice := NewAccount(aliceAddr, pubKey)
	alice.AddBalance(types.NewAmount(1000))
	s.SetAccount(aliceAddr, alice)
	s.SetAccount(bobAddr, NewAccount(bobAddr, pubKey))

	err := Transfer(s, aliceAddr, bobAddr, types.NewAmount(500), hasher)
	if err != nil {
		t.Fatal(err)
	}

	alice, _ = s.GetAccount(aliceAddr)
	bob, _ := s.GetAccount(bobAddr)

	if alice.Balance.Cmp(types.NewAmount(500)) != 0 {
		t.Errorf("alice balance = %s, want 500", alice.Balance)
	}
	if bob.Balance.Cmp(types.NewAmount(500)) != 0 {
		t.Errorf("bob balance = %s, want 500", bob.Balance)
	}
}

func TestTransfer_InsufficientBalance(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s := NewInMemoryState(hasher)

	aliceAddr := types.Address([20]byte{1})
	bobAddr := types.Address([20]byte{2})

	alice := NewAccount(aliceAddr, [PublicKeySize]byte{})
	alice.AddBalance(types.NewAmount(100))
	s.SetAccount(aliceAddr, alice)
	s.SetAccount(bobAddr, NewAccount(bobAddr, [PublicKeySize]byte{}))

	err := Transfer(s, aliceAddr, bobAddr, types.NewAmount(200), hasher)
	if err == nil {
		t.Fatal("expected insufficient balance error")
	}
}

func TestTransfer_SenderNotFound(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s := NewInMemoryState(hasher)

	bobAddr := types.Address([20]byte{2})
	s.SetAccount(bobAddr, NewAccount(bobAddr, [PublicKeySize]byte{}))

	err := Transfer(s, types.Address([20]byte{1}), bobAddr, types.NewAmount(100), hasher)
	if err == nil {
		t.Fatal("expected error for non-existent sender")
	}
}

func TestTransfer_SelfTransfer(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s := NewInMemoryState(hasher)

	addr := types.Address([20]byte{1})
	acc := NewAccount(addr, [PublicKeySize]byte{})
	acc.AddBalance(types.NewAmount(500))
	s.SetAccount(addr, acc)

	err := Transfer(s, addr, addr, types.NewAmount(100), hasher)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetAccount(addr)
	if got.Balance.Cmp(types.NewAmount(500)) != 0 {
		t.Errorf("self transfer should not change balance, got %s", got.Balance)
	}
}

func TestMint(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s := NewInMemoryState(hasher)

	addr := types.Address([20]byte{1})
	s.SetAccount(addr, NewAccount(addr, [PublicKeySize]byte{}))

	err := Mint(s, addr, types.NewAmount(1000), hasher)
	if err != nil {
		t.Fatal(err)
	}

	acc, _ := s.GetAccount(addr)
	if acc.Balance.Cmp(types.NewAmount(1000)) != 0 {
		t.Errorf("balance = %s, want 1000", acc.Balance)
	}
}

func TestBurn(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s := NewInMemoryState(hasher)

	addr := types.Address([20]byte{1})
	acc := NewAccount(addr, [PublicKeySize]byte{})
	acc.AddBalance(types.NewAmount(1000))
	s.SetAccount(addr, acc)

	err := Burn(s, addr, types.NewAmount(300), hasher)
	if err != nil {
		t.Fatal(err)
	}

	got, _ := s.GetAccount(addr)
	if got.Balance.Cmp(types.NewAmount(700)) != 0 {
		t.Errorf("balance = %s, want 700", got.Balance)
	}
}

func TestBurnInsufficientBalance(t *testing.T) {
	hasher := types.SHA256Hasher{}
	s := NewInMemoryState(hasher)

	addr := types.Address([20]byte{1})
	acc := NewAccount(addr, [PublicKeySize]byte{})
	acc.AddBalance(types.NewAmount(100))
	s.SetAccount(addr, acc)

	err := Burn(s, addr, types.NewAmount(200), hasher)
	if err == nil {
		t.Fatal("expected insufficient balance error")
	}
}
