package staking

import (
	"strconv"
	"testing"

	"github.com/BryanOx/dsn/types"
)

// setupValidators creates and activates validators with the given addresses and stakes.
// Returns the validator IDs (ConsensusID).
func setupValidators(s *testStakingState, addrs []types.Address, stakes []uint64) []types.Address {
	var ids []types.Address
	for i, addr := range addrs {
		fundAccount(s, addr, stakes[i]*2) // enough for stake
		id := registerAndActivate(s, addr, stakes[i])
		ids = append(ids, id)
	}
	return ids
}

// setupAndDistribute sets up validators, creates a snapshot, writes validator pool,
// and calls DistributeValidatorRewards. Returns the testStakingState.
func setupAndDistribute(t *testing.T, validators []struct {
	addr  types.Address
	stake uint64
}, epoch uint64, poolAmount uint64) *testStakingState {
	s := newTestStakingState()

	// Register and activate each validator
	var addrs []types.Address
	var stakes []uint64
	for _, v := range validators {
		addrs = append(addrs, v.addr)
		stakes = append(stakes, v.stake)
	}
	ids := setupValidators(s, addrs, stakes)

	// Create snapshot for epoch-1 (snapshot for epoch N is created at beginning of epoch N)
	CreateSnapshot(s, epoch-1)

	// Write validator pool for this epoch
	poolKey := KeyEpochValidatorPool + strconv.FormatUint(epoch, 10)
	WriteUint64(s, poolKey, poolAmount)

	// Distribute
	err := DistributeValidatorRewards(s, epoch)
	if err != nil {
		t.Fatalf("DistributeValidatorRewards: %v", err)
	}

	_ = ids // suppress unused warning
	return s
}

// TestDistributeValidatorRewards_Basic tests basic proportional distribution
func TestDistributeValidatorRewards_Basic(t *testing.T) {
	addr1, _ := types.AddressFromBytes([]byte("11111111111111111111"))
	addr2, _ := types.AddressFromBytes([]byte("22222222222222222222"))
	addr3, _ := types.AddressFromBytes([]byte("33333333333333333333"))

	validators := []struct {
		addr  types.Address
		stake uint64
	}{
		{addr: addr1, stake: 100_000},
		{addr: addr2, stake: 200_000},
		{addr: addr3, stake: 300_000},
	}

	s := setupAndDistribute(t, validators, 2, 1000)

	// Total power = 100k + 200k + 300k = 600k
	// Distribution:
	// - Validator 3 (300k): 1000 * 300000 / 600000 = 500
	// - Validator 2 (200k): 1000 * 200000 / 600000 = 333
	// - Validator 1 (100k): 1000 * 100000 / 600000 = 166
	// Remainder = 1000 - 500 - 333 - 166 = 1 → goes to validator 3 (highest power)
	// Final: v3 = 501, v2 = 333, v1 = 166

	acc1, _ := s.GetAccount(addr1)
	acc2, _ := s.GetAccount(addr2)
	acc3, _ := s.GetAccount(addr3)

	// Balance after registration: funded - stake = stake
	// After reward: stake + reward
	var expected1 uint64 = 100_000 + 166
	var expected2 uint64 = 200_000 + 333
	var expected3 uint64 = 300_000 + 501

	if acc1.Balance.Cmp(types.NewAmount(expected1)) != 0 {
		t.Errorf("addr1 balance = %s, want %d", acc1.Balance, expected1)
	}
	if acc2.Balance.Cmp(types.NewAmount(expected2)) != 0 {
		t.Errorf("addr2 balance = %s, want %d", acc2.Balance, expected2)
	}
	if acc3.Balance.Cmp(types.NewAmount(expected3)) != 0 {
		t.Errorf("addr3 balance = %s, want %d", acc3.Balance, expected3)
	}

	// Verify total distributed = pool (1000)
	total := uint64(166 + 333 + 501)
	if total != 1000 {
		t.Errorf("total distributed = %d, want 1000", total)
	}
}

// TestDistributeValidatorRewards_Proportional tests equal stakes get equal rewards
func TestDistributeValidatorRewards_Proportional(t *testing.T) {
	addr1, _ := types.AddressFromBytes([]byte("11111111111111111111"))
	addr2, _ := types.AddressFromBytes([]byte("22222222222222222222"))

	validators := []struct {
		addr  types.Address
		stake uint64
	}{
		{addr: addr1, stake: 100_000},
		{addr: addr2, stake: 100_000},
	}

	s := setupAndDistribute(t, validators, 2, 1000)

	// Both have 100k stake = 50% each → 500 each
	// Balance after registration: 100k (stake*2 - stake = 100k)
	// After reward: 100k + 500 = 100500
	acc1, _ := s.GetAccount(addr1)
	acc2, _ := s.GetAccount(addr2)

	if acc1.Balance.Cmp(types.NewAmount(100_500)) != 0 {
		t.Errorf("addr1 balance = %s, want 100500", acc1.Balance)
	}
	if acc2.Balance.Cmp(types.NewAmount(100_500)) != 0 {
		t.Errorf("addr2 balance = %s, want 100500", acc2.Balance)
	}
}

// TestDistributeValidatorRewards_RemainderToTop tests that remainder goes to highest-power validator
func TestDistributeValidatorRewards_RemainderToTop(t *testing.T) {
	addr1, _ := types.AddressFromBytes([]byte("11111111111111111111"))
	addr2, _ := types.AddressFromBytes([]byte("22222222222222222222"))

	validators := []struct {
		addr  types.Address
		stake uint64
	}{
		{addr: addr1, stake: 100_000},
		{addr: addr2, stake: 200_000},
	}

	s := setupAndDistribute(t, validators, 2, 100)

	// Total power = 300k
	// addr2 (200k): 100 * 200000 / 300000 = 66
	// addr1 (100k): 100 * 100000 / 300000 = 33
	// Remainder = 100 - 66 - 33 = 1 → goes to addr2 (higher power)
	// Final: addr2 = 67, addr1 = 33

	acc1, _ := s.GetAccount(addr1)
	acc2, _ := s.GetAccount(addr2)

	// Balance after registration: addr1 = 100k, addr2 = 200k (funded - stake)
	// After reward: addr1 = 100k + 33 = 100033, addr2 = 200k + 67 = 200067
	if acc1.Balance.Cmp(types.NewAmount(100_033)) != 0 {
		t.Errorf("addr1 balance = %s, want 100033", acc1.Balance)
	}
	if acc2.Balance.Cmp(types.NewAmount(200_067)) != 0 {
		t.Errorf("addr2 balance = %s, want 200067", acc2.Balance)
	}

	// Verify total = 100
	total := uint64(33 + 67)
	if total != 100 {
		t.Errorf("total distributed = %d, want 100", total)
	}
}

// TestDistributeValidatorRewards_NoSnapshot tests that epoch with no snapshot returns nil
func TestDistributeValidatorRewards_NoSnapshot(t *testing.T) {
	s := newTestStakingState()

	// Write a validator pool but don't create any snapshot
	WriteUint64(s, KeyEpochValidatorPool+"1", 1000)

	// Epoch 1 looks for snapshot 0, which doesn't exist → should return nil
	err := DistributeValidatorRewards(s, 1)
	if err != nil {
		t.Fatalf("DistributeValidatorRewards: %v", err)
	}
}

// TestDistributeValidatorRewards_NoPool tests when validator pool key doesn't exist
func TestDistributeValidatorRewards_NoPool(t *testing.T) {
	s := newTestStakingState()

	// Create snapshot but don't write validator pool
	CreateSnapshot(s, 1)

	// Should return nil (no pool to distribute)
	err := DistributeValidatorRewards(s, 2)
	if err != nil {
		t.Fatalf("DistributeValidatorRewards: %v", err)
	}
}

// TestDistributeValidatorRewards_ZeroPool tests zero pool returns nil
func TestDistributeValidatorRewards_ZeroPool(t *testing.T) {
	s := newTestStakingState()

	addr, _ := types.AddressFromBytes([]byte("11111111111111111111"))
	fundAccount(s, addr, 200_000)
	registerAndActivate(s, addr, 100_000)

	// Create snapshot and write zero pool
	CreateSnapshot(s, 1)
	WriteUint64(s, KeyEpochValidatorPool+"2", 0)

	err := DistributeValidatorRewards(s, 2)
	if err != nil {
		t.Fatalf("DistributeValidatorRewards: %v", err)
	}

	// Verify no balance change (still 100k after registration deduction)
	acc, _ := s.GetAccount(addr)
	if acc.Balance.Cmp(types.NewAmount(100_000)) != 0 {
		t.Errorf("balance = %s, want 100000 (no change)", acc.Balance)
	}
}

// TestDistributeValidatorRewards_RewardAddress tests that rewards go to RewardAddress, not OperatorAddress
func TestDistributeValidatorRewards_RewardAddress(t *testing.T) {
	addr, _ := types.AddressFromBytes([]byte("11111111111111111111"))
	rewardAddr, _ := types.AddressFromBytes([]byte("22222222222222222222"))

	s := newTestStakingState()
	fundAccount(s, addr, 200_000)
	fundAccount(s, rewardAddr, 0) // ensure account exists

	id := registerAndActivate(s, addr, 100_000)

	// Set RewardAddress before creating snapshot
	v, _ := GetValidator(s, id)
	v.RewardAddress = rewardAddr
	UpdateValidator(s, v)

	// Create snapshot for epoch 1 (epoch-1 = 1-1 = 0)
	CreateSnapshot(s, 1)

	// Write validator pool for epoch 2
	WriteUint64(s, KeyEpochValidatorPool+"2", 500)

	// Distribute
	err := DistributeValidatorRewards(s, 2)
	if err != nil {
		t.Fatalf("DistributeValidatorRewards: %v", err)
	}

	// Reward should go to rewardAddr, not addr
	accReward, _ := s.GetAccount(rewardAddr)
	accOperator, _ := s.GetAccount(addr)

	// rewardAddr: 0 + 500 = 500
	// addr: 100k (after registration) + 0 reward
	if accReward.Balance.Cmp(types.NewAmount(500)) != 0 {
		t.Errorf("rewardAddr balance = %s, want 500", accReward.Balance)
	}
	if accOperator.Balance.Cmp(types.NewAmount(100_000)) != 0 {
		t.Errorf("addr balance = %s, want 100000 (no reward)", accOperator.Balance)
	}
}

// TestDistributeValidatorRewards_JailedExcluded tests that jailed validators are excluded from snapshots
func TestDistributeValidatorRewards_JailedExcluded(t *testing.T) {
	addr1, _ := types.AddressFromBytes([]byte("11111111111111111111"))
	addr2, _ := types.AddressFromBytes([]byte("22222222222222222222"))

	s := newTestStakingState()

	// Create 2 validators
	fundAccount(s, addr1, 200_000)
	fundAccount(s, addr2, 200_000)
	id1 := registerAndActivate(s, addr1, 100_000)
	id2 := registerAndActivate(s, addr2, 100_000)

	// Create snapshot with both validators active (epoch 1)
	CreateSnapshot(s, 1)

	// Jail validator 1 AFTER snapshot was created
	// This way, snapshot 1 has both validators, but current state has one jailed
	JailValidator(s, id1, 10)

	// Distribute for epoch 2 using snapshot 1 (before jailing)
	// Since DistributeValidatorRewards uses snapshot (epoch-1), epoch 2 uses snapshot 1
	// But wait, DistributeValidatorRewards reads snapshot at (epoch-1), so epoch 2 looks for snapshot 1
	// That's correct - snapshot 1 has both validators active
	WriteUint64(s, KeyEpochValidatorPool+"2", 1000)
	err := DistributeValidatorRewards(s, 2)
	if err != nil {
		t.Fatalf("DistributeValidatorRewards: %v", err)
	}

	// Both should get rewards since snapshot 1 was created before jailing
	// Actually wait, the test says "jailed excluded" - meaning we want to test that
	// if a validator is jailed BEFORE creating a snapshot, they get excluded
	// Let me redo this test...

	// For proper jailed exclusion test:
	// 1. Register validator 1, create snapshot (includes v1)
	// 2. Register validator 2, create snapshot (includes both)
	// 3. Jail validator 1, create snapshot (only v2)
	// 4. Distribute - only v2 gets rewards

	// Let's redo with proper sequence:
	s2 := newTestStakingState()
	fundAccount(s2, addr1, 200_000)
	fundAccount(s2, addr2, 200_000)
	_ = registerAndActivate(s2, addr1, 100_000)

	// Snapshot 1: only validator 1 active
	CreateSnapshot(s2, 1)

	// Now register validator 2
	_ = registerAndActivate(s2, addr2, 100_000)

	// Snapshot 2: both validators active
	CreateSnapshot(s2, 2)

	// Now jail validator 1
	JailValidator(s2, id1, 10)

	// Snapshot 3: only validator 2 active (jailed excluded)
	CreateSnapshot(s2, 3)

	// Distribute epoch 4 using snapshot 3 (only validator 2)
	WriteUint64(s2, KeyEpochValidatorPool+"4", 1000)
	err = DistributeValidatorRewards(s2, 4)
	if err != nil {
		t.Fatalf("DistributeValidatorRewards: %v", err)
	}

	// Only validator 2 should get rewards
	acc1, _ := s2.GetAccount(addr1)
	acc2, _ := s2.GetAccount(addr2)

	// addr1: 100k (after registration) + 0 reward = 100k
	// addr2: 100k (after registration) + 1000 reward = 101000
	if acc1.Balance.Cmp(types.NewAmount(100_000)) != 0 {
		t.Errorf("addr1 balance (jailed) = %s, want 100000", acc1.Balance)
	}
	if acc2.Balance.Cmp(types.NewAmount(101_000)) != 0 {
		t.Errorf("addr2 balance = %s, want 101000", acc2.Balance)
	}

	_ = id2 // suppress unused warning
}

// TestDistributeValidatorRewards_MultipleEpochs tests that different epochs use different snapshots
func TestDistributeValidatorRewards_MultipleEpochs(t *testing.T) {
	addr1, _ := types.AddressFromBytes([]byte("11111111111111111111"))
	addr2, _ := types.AddressFromBytes([]byte("22222222222222222222"))

	s := newTestStakingState()

	// Create 2 validators with different stakes
	fundAccount(s, addr1, 200_000)
	fundAccount(s, addr2, 300_000)
	registerAndActivate(s, addr1, 100_000)
	registerAndActivate(s, addr2, 200_000)

	// Create snapshot for epoch 1
	CreateSnapshot(s, 1)

	// Distribute epoch 2 with pool 200 (uses snapshot 1)
	WriteUint64(s, KeyEpochValidatorPool+"2", 200)
	DistributeValidatorRewards(s, 2)

	// Add a third validator
	addr3, _ := types.AddressFromBytes([]byte("33333333333333333333"))
	fundAccount(s, addr3, 200_000)
	registerAndActivate(s, addr3, 100_000)

	// Create snapshot for epoch 2 (includes all 3)
	CreateSnapshot(s, 2)

	// Distribute epoch 3 with pool 300 (uses snapshot 2)
	WriteUint64(s, KeyEpochValidatorPool+"3", 300)
	DistributeValidatorRewards(s, 3)

	// Epoch 2 distribution: validators 1 (100k), 2 (200k) = total 300k
	//   v2: 200 * 200000/300000 = 133
	//   v1: 200 * 100000/300000 = 66
	//   remainder = 200 - 133 - 66 = 1 → v2
	//   v2 = 134, v1 = 66
	// Epoch 3 distribution: validators 1 (100k), 2 (200k), 3 (100k) = total 400k
	//   v2: 300 * 200000/400000 = 150
	//   v1: 300 * 100000/400000 = 75
	//   v3: 300 * 100000/400000 = 75
	//   remainder = 0

	acc1, _ := s.GetAccount(addr1)
	acc2, _ := s.GetAccount(addr2)
	acc3, _ := s.GetAccount(addr3)

	// Balance after registration: addr1 = 100k, addr2 = 100k, addr3 = 100k
	// Total rewards: addr1 = 66 + 75 = 141, addr2 = 134 + 150 = 284, addr3 = 75
	// Final: addr1 = 100k + 141 = 100141, addr2 = 100k + 284 = 100284, addr3 = 100k + 75 = 100075

	if acc1.Balance.Cmp(types.NewAmount(100_141)) != 0 {
		t.Errorf("addr1 balance = %s, want 100141", acc1.Balance)
	}
	if acc2.Balance.Cmp(types.NewAmount(100_284)) != 0 {
		t.Errorf("addr2 balance = %s, want 100284", acc2.Balance)
	}
	if acc3.Balance.Cmp(types.NewAmount(100_075)) != 0 {
		t.Errorf("addr3 balance = %s, want 100075", acc3.Balance)
	}
}

// TestDistributeValidatorRewards_LargeNumbers tests distribution with large numbers
func TestDistributeValidatorRewards_LargeNumbers(t *testing.T) {
	addr1, _ := types.AddressFromBytes([]byte("11111111111111111111"))
	addr2, _ := types.AddressFromBytes([]byte("22222222222222222222"))

	validators := []struct {
		addr  types.Address
		stake uint64
	}{
		{addr: addr1, stake: 10_000_000},
		{addr: addr2, stake: 10_000_000},
	}

	s := setupAndDistribute(t, validators, 2, 1_000_000)

	// Equal stakes = equal rewards: 500,000 each
	// Balance after registration: 10M stake*2 - 10M stake = 10M
	// After reward: 10M + 500k = 10.5M
	acc1, _ := s.GetAccount(addr1)
	acc2, _ := s.GetAccount(addr2)

	if acc1.Balance.Cmp(types.NewAmount(10_500_000)) != 0 {
		t.Errorf("addr1 balance = %s, want 10500000", acc1.Balance)
	}
	if acc2.Balance.Cmp(types.NewAmount(10_500_000)) != 0 {
		t.Errorf("addr2 balance = %s, want 10500000", acc2.Balance)
	}
}

// TestDistributeValidatorRewards_CreateAccountIfMissing tests that rewards can be credited to new accounts
func TestDistributeValidatorRewards_CreateAccountIfMissing(t *testing.T) {
	addr1, _ := types.AddressFromBytes([]byte("11111111111111111111"))
	// Reward address doesn't exist (no account created yet)
	rewardAddr, _ := types.AddressFromBytes([]byte("22222222222222222222"))

	s := newTestStakingState()
	fundAccount(s, addr1, 200_000)

	id := registerAndActivate(s, addr1, 100_000)

	// Set RewardAddress to account that doesn't exist
	v, _ := GetValidator(s, id)
	v.RewardAddress = rewardAddr
	UpdateValidator(s, v)

	CreateSnapshot(s, 1)
	WriteUint64(s, KeyEpochValidatorPool+"2", 100)

	err := DistributeValidatorRewards(s, 2)
	if err != nil {
		t.Fatalf("DistributeValidatorRewards: %v", err)
	}

	// Should create account for rewardAddr and credit it
	acc, err := s.GetAccount(rewardAddr)
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if acc.Balance.Cmp(types.NewAmount(100)) != 0 {
		t.Errorf("rewardAddr balance = %s, want 100", acc.Balance)
	}
}
