package staking

import (
	"testing"

	"github.com/BryanOx/dsn/state"
	"github.com/BryanOx/dsn/types"
)

// testStakingState implements StakingState using the real state package.
type testStakingState struct {
	*testKVStore
	st *state.InMemoryState
}

func newTestStakingState() *testStakingState {
	hasher := types.SHA256Hasher{}
	return &testStakingState{
		testKVStore: newTestKVStore(),
		st:          state.NewInMemoryState(hasher),
	}
}

func (s *testStakingState) GetAccount(addr types.Address) (*state.Account, error) {
	return s.st.GetAccount(addr)
}

func (s *testStakingState) SetAccount(addr types.Address, account *state.Account) error {
	return s.st.SetAccount(addr, account)
}

func fundAccount(s *testStakingState, addr types.Address, amount uint64) {
	acc := state.NewAccount(addr, [32]byte{})
	acc.AddBalance(types.NewAmount(amount))
	s.st.SetAccount(addr, acc)
}

func registerAndActivate(s *testStakingState, addr types.Address, stake uint64) types.Address {
	pubKey := [32]byte{}
	copy(pubKey[:], addr.Bytes())
	cid, _ := RegisterValidator(s, pubKey, addr, types.NewAmount(stake), 0, 0)
	ActivateValidator(s, cid, 1)
	// The caller must deduct the bonded amount from the account
	acc, _ := s.GetAccount(addr)
	acc.SubBalance(types.NewAmount(stake))
	s.SetAccount(addr, acc)
	return cid
}

func TestBond_Valid(t *testing.T) {
	s := newTestStakingState()
	addr, _ := types.AddressFromBytes([]byte("11111111111111111111"))
	fundAccount(s, addr, 200_000)

	id := registerAndActivate(s, addr, 100_000)

	newStake, err := Bond(s, addr, id, types.NewAmount(50_000))
	if err != nil {
		t.Fatalf("Bond failed: %v", err)
	}

	// Check stake increased
	if newStake.Cmp(types.NewAmount(150_000)) != 0 {
		t.Errorf("stake = %s, want 150000", newStake)
	}

	// Check balance decreased (200k funded - 100k registered - 50k bonded)
	acc, _ := s.GetAccount(addr)
	if acc.Balance.Cmp(types.NewAmount(50_000)) != 0 {
		t.Errorf("balance = %s, want 50000 (200k funded - 100k registered - 50k bonded)", acc.Balance)
	}

	// Check total bonded
	total, _ := TotalBonded(s)
	if total.Cmp(types.NewAmount(150_000)) != 0 {
		t.Errorf("total bonded = %s, want 150000", total)
	}
}

func TestBond_InsufficientBalance(t *testing.T) {
	s := newTestStakingState()
	addr, _ := types.AddressFromBytes([]byte("11111111111111111111"))
	fundAccount(s, addr, 50_000)

	id := registerAndActivate(s, addr, 100_000)

	_, err := Bond(s, addr, id, types.NewAmount(100_000))
	if err == nil {
		t.Fatal("expected error for insufficient balance")
	}
}

func TestBond_NonExistentValidator(t *testing.T) {
	s := newTestStakingState()
	addr, _ := types.AddressFromBytes([]byte("11111111111111111111"))
	fundAccount(s, addr, 200_000)

	nonExistentID := types.Address{9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9}
	_, err := Bond(s, addr, nonExistentID, types.NewAmount(50_000))
	if err == nil {
		t.Fatal("expected error for non-existent validator")
	}
}

func TestSlash_Valid(t *testing.T) {
	s := newTestStakingState()
	addr, _ := types.AddressFromBytes([]byte("11111111111111111111"))
	fundAccount(s, addr, 200_000)

	id := registerAndActivate(s, addr, 100_000)

	slashed, err := Slash(s, id, types.NewAmount(30_000))
	if err != nil {
		t.Fatalf("Slash failed: %v", err)
	}
	if slashed.Cmp(types.NewAmount(30_000)) != 0 {
		t.Errorf("slashed = %s, want 30000", slashed)
	}

	// Check stake decreased (100k - 30k = 70k)
	v, _ := GetValidator(s, id)
	if v.BondedStake.Cmp(types.NewAmount(70_000)) != 0 {
		t.Errorf("stake after slash = %s, want 70000", v.BondedStake)
	}

	// Check total bonded decreased
	total, _ := TotalBonded(s)
	if total.Cmp(types.NewAmount(70_000)) != 0 {
		t.Errorf("total bonded = %s, want 70000", total)
	}

	// Account balance unchanged by slash (slash only affects stake)
	acc, _ := s.GetAccount(addr)
	if acc.Balance.Cmp(types.NewAmount(100_000)) != 0 {
		t.Errorf("account balance = %s, want 100000 (200k - 100k registered)", acc.Balance)
	}
}

func TestSlash_MaximumAmount(t *testing.T) {
	s := newTestStakingState()
	addr, _ := types.AddressFromBytes([]byte("11111111111111111111"))
	fundAccount(s, addr, 200_000)

	id := registerAndActivate(s, addr, 100_000)

	// Slash more than bonded stake
	slashed, err := Slash(s, id, types.NewAmount(500_000))
	if err != nil {
		t.Fatalf("Slash failed: %v", err)
	}
	// Should have slashed only what's available
	if slashed.Cmp(types.NewAmount(100_000)) != 0 {
		t.Errorf("slashed = %s, want 100000 (max bonded)", slashed)
	}

	v, _ := GetValidator(s, id)
	if !v.BondedStake.IsZero() {
		t.Errorf("stake = %s, want 0 after full slash", v.BondedStake)
	}

	// Account balance unchanged by slash
	acc, _ := s.GetAccount(addr)
	if acc.Balance.Cmp(types.NewAmount(100_000)) != 0 {
		t.Errorf("account balance = %s, want 100000", acc.Balance)
	}
}

func TestReleaseStake_Valid(t *testing.T) {
	s := newTestStakingState()
	addr, _ := types.AddressFromBytes([]byte("11111111111111111111"))
	fundAccount(s, addr, 200_000)

	id := registerAndActivate(s, addr, 100_000)
	StartUnstake(s, id, 14)

	// Balance before release: 200k funded - 100k registered = 100k
	acc, _ := s.GetAccount(addr)
	beforeBalance := acc.Balance
	if beforeBalance.Cmp(types.NewAmount(100_000)) != 0 {
		t.Fatalf("before balance = %s, want 100000", beforeBalance)
	}

	if err := ReleaseStake(s, id); err != nil {
		t.Fatalf("ReleaseStake failed: %v", err)
	}

	// Check balance increased by bonded stake (100k + 100k = 200k)
	acc, _ = s.GetAccount(addr)
	expected := types.NewAmount(stakeToUint64(beforeBalance) + 100_000)
	if acc.Balance.Cmp(expected) != 0 {
		t.Errorf("balance = %s, want %s", acc.Balance, expected)
	}

	// Check total bonded
	total, _ := TotalBonded(s)
	if !total.IsZero() {
		t.Errorf("total bonded = %s, want 0 after release", total)
	}
}

func TestReleaseStake_NotUnstaking(t *testing.T) {
	s := newTestStakingState()
	addr, _ := types.AddressFromBytes([]byte("11111111111111111111"))
	fundAccount(s, addr, 200_000)

	id := registerAndActivate(s, addr, 100_000)

	err := ReleaseStake(s, id)
	if err == nil {
		t.Fatal("expected error for releasing non-unstaking validator")
	}
}

func TestDistributeRewards_Empty(t *testing.T) {
	s := newTestStakingState()
	vShare, bShare, tShare, err := DistributeRewards(s, types.NewAmount(0))
	if err != nil {
		t.Fatal(err)
	}
	if !vShare.IsZero() || !bShare.IsZero() || !tShare.IsZero() {
		t.Error("all shares should be zero for zero fees")
	}
}

func TestDistributeRewards_Basic(t *testing.T) {
	s := newTestStakingState()
	addr1, _ := types.AddressFromBytes([]byte("11111111111111111111"))
	addr2, _ := types.AddressFromBytes([]byte("22222222222222222222"))
	fundAccount(s, addr1, 200_000)
	fundAccount(s, addr2, 200_000)

	registerAndActivate(s, addr1, 100_000)
	registerAndActivate(s, addr2, 100_000)

	totalFees := types.NewAmount(1000)
	vShare, bShare, tShare, err := DistributeRewards(s, totalFees)
	if err != nil {
		t.Fatalf("DistributeRewards failed: %v", err)
	}

	// 70% to validators = 700
	if vShare.Cmp(types.NewAmount(700)) != 0 {
		t.Errorf("validator share = %s, want 700", vShare)
	}
	// 20% burn = 200
	if bShare.Cmp(types.NewAmount(200)) != 0 {
		t.Errorf("burn share = %s, want 200", bShare)
	}
	// 10% treasury = 100
	if tShare.Cmp(types.NewAmount(100)) != 0 {
		t.Errorf("treasury share = %s, want 100", tShare)
	}

	// Each validator should get 350 (700 / 2)
	acc1, _ := s.GetAccount(addr1)
	if acc1.Balance.Cmp(types.NewAmount(100_000+350)) != 0 {
		t.Errorf("validator1 balance = %s, want 100350", acc1.Balance)
	}
	acc2, _ := s.GetAccount(addr2)
	if acc2.Balance.Cmp(types.NewAmount(100_000+350)) != 0 {
		t.Errorf("validator2 balance = %s, want 100350", acc2.Balance)
	}
}

func TestDistributeRewards_Proportional(t *testing.T) {
	s := newTestStakingState()
	addr1, _ := types.AddressFromBytes([]byte("11111111111111111111"))
	addr2, _ := types.AddressFromBytes([]byte("22222222222222222222"))
	fundAccount(s, addr1, 300_000)
	fundAccount(s, addr2, 200_000)

	// 200k vs 100k stake → 2:1 voting power
	registerAndActivate(s, addr1, 200_000)
	registerAndActivate(s, addr2, 100_000)

	totalFees := types.NewAmount(1000)
	DistributeRewards(s, totalFees)

	// Validator share = 700. v1 gets 2/3, v2 gets 1/3.
	// 700 * 200000 / 300000 = 466, 700 * 100000 / 300000 = 233
	// Remainder (1) goes to top validator → 467, 233
	//
	// Balances after registration deduction:
	//   v1: 300k funded - 200k registered = 100k
	//   v2: 200k funded - 100k registered = 100k
	acc1, _ := s.GetAccount(addr1)
	acc2, _ := s.GetAccount(addr2)

	// Account for remainder going to top validator
	if acc1.Balance.Cmp(types.NewAmount(100_000+467)) != 0 &&
		acc1.Balance.Cmp(types.NewAmount(100_000+466)) != 0 {
		t.Errorf("v1 balance = %s, want 100467 or 100466", acc1.Balance)
	}
	if acc2.Balance.Cmp(types.NewAmount(100_000+233)) != 0 {
		t.Errorf("v2 balance = %s, want 100233", acc2.Balance)
	}
}

func TestBondThenSlashSequence(t *testing.T) {
	s := newTestStakingState()
	addr, _ := types.AddressFromBytes([]byte("11111111111111111111"))
	fundAccount(s, addr, 500_000)

	id := registerAndActivate(s, addr, 100_000)

	// Balance after registration: 500k - 100k = 400k

	// Bond more (50k from balance)
	Bond(s, addr, id, types.NewAmount(50_000))

	// Balance: 400k - 50k = 350k

	// Slash
	Slash(s, id, types.NewAmount(30_000))

	v, _ := GetValidator(s, id)
	// Initial 100k + 50k bond - 30k slash = 120k
	if v.BondedStake.Cmp(types.NewAmount(120_000)) != 0 {
		t.Errorf("final stake = %s, want 120000", v.BondedStake)
	}

	// Check total bonded
	total, _ := TotalBonded(s)
	if total.Cmp(types.NewAmount(120_000)) != 0 {
		t.Errorf("total bonded = %s, want 120000", total)
	}

	// Account balance: 500k - 100k (register) - 50k (bond) = 350k (slash doesn't affect balance)
	acc, _ := s.GetAccount(addr)
	if acc.Balance.Cmp(types.NewAmount(350_000)) != 0 {
		t.Errorf("account balance = %s, want 350000", acc.Balance)
	}
}
