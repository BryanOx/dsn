package staking

import (
	"testing"

	"github.com/dsn/dsn/types"
)

func TestGetTreasuryBalance_Default(t *testing.T) {
	s := newTestStakingState()

	balance, err := GetTreasuryBalance(s)
	if err != nil {
		t.Fatalf("GetTreasuryBalance failed: %v", err)
	}

	if balance != 0 {
		t.Errorf("balance = %d, want 0 for unfunded treasury", balance)
	}
}

func TestCreditTreasury_AccountBalance(t *testing.T) {
	s := newTestStakingState()

	// Credit treasury with 100_000
	if err := CreditTreasury(s, 100_000); err != nil {
		t.Fatalf("CreditTreasury(100_000) failed: %v", err)
	}

	// Check balance via GetTreasuryBalance
	balance, err := GetTreasuryBalance(s)
	if err != nil {
		t.Fatalf("GetTreasuryBalance failed: %v", err)
	}
	if balance != 100_000 {
		t.Errorf("treasury balance = %d, want 100000", balance)
	}

	// Check account balance matches
	acc, err := s.GetAccount(TreasuryAddress)
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if acc.Balance.Cmp(types.NewAmount(100_000)) != 0 {
		t.Errorf("account balance = %s, want 100000", acc.Balance)
	}

	// Credit another 50_000
	if err := CreditTreasury(s, 50_000); err != nil {
		t.Fatalf("CreditTreasury(50_000) failed: %v", err)
	}

	// Check updated balance
	balance, err = GetTreasuryBalance(s)
	if err != nil {
		t.Fatalf("GetTreasuryBalance failed: %v", err)
	}
	if balance != 150_000 {
		t.Errorf("treasury balance = %d, want 150000", balance)
	}

	// Check updated account balance
	acc, err = s.GetAccount(TreasuryAddress)
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if acc.Balance.Cmp(types.NewAmount(150_000)) != 0 {
		t.Errorf("account balance = %s, want 150000", acc.Balance)
	}
}

func TestCreditTreasury_Cumulative(t *testing.T) {
	s := newTestStakingState()

	// Credit 777 twice
	if err := CreditTreasury(s, 777); err != nil {
		t.Fatalf("CreditTreasury(777) failed: %v", err)
	}
	if err := CreditTreasury(s, 777); err != nil {
		t.Fatalf("CreditTreasury(777) failed: %v", err)
	}

	// Balance should be 1554
	balance, err := GetTreasuryBalance(s)
	if err != nil {
		t.Fatalf("GetTreasuryBalance failed: %v", err)
	}
	if balance != 1554 {
		t.Errorf("balance = %d, want 1554", balance)
	}

	// Account balance should match
	acc, err := s.GetAccount(TreasuryAddress)
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if acc.Balance.Cmp(types.NewAmount(1554)) != 0 {
		t.Errorf("account balance = %s, want 1554", acc.Balance)
	}
}

func TestTreasuryAddress_IsZero(t *testing.T) {
	if TreasuryAddress != (types.Address{}) {
		t.Errorf("TreasuryAddress = %v, want zero address", TreasuryAddress)
	}
}
