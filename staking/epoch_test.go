package staking

import (
	"testing"

	"github.com/dsn/dsn/types"
)

func TestEpochAtHeight(t *testing.T) {
	s := newTestKVStore()

	tests := []struct {
		height uint64
		epoch  uint64
	}{
		{0, 0},     // genesis
		{1, 1},     // first block of epoch 1
		{99, 1},    // last block of epoch 1
		{100, 1},   // last block of epoch 1
		{101, 2},   // first block of epoch 2
		{200, 2},   // last block of epoch 2
		{201, 3},   // first block of epoch 3
		{1000, 10}, // block 1000 → epoch 10
	}
	for _, tt := range tests {
		got := EpochAtHeight(s, tt.height)
		if got != tt.epoch {
			t.Errorf("EpochAtHeight(%d) = %d, want %d", tt.height, got, tt.epoch)
		}
	}
}

func TestEpochAtHeight_CustomBlocksPerEpoch(t *testing.T) {
	s := newTestKVStore()
	SetBlocksPerEpoch(s, 50)

	if e := EpochAtHeight(s, 0); e != 0 {
		t.Errorf("genesis epoch = %d, want 0", e)
	}
	if e := EpochAtHeight(s, 1); e != 1 {
		t.Errorf("block 1 epoch = %d, want 1", e)
	}
	if e := EpochAtHeight(s, 50); e != 1 {
		t.Errorf("block 50 epoch = %d, want 1 (last block of epoch 1)", e)
	}
	if e := EpochAtHeight(s, 51); e != 2 {
		t.Errorf("block 51 epoch = %d, want 2", e)
	}
	if e := EpochAtHeight(s, 100); e != 2 {
		t.Errorf("block 100 epoch = %d, want 2 (last block of epoch 2)", e)
	}
}

func TestIsEpochBoundary(t *testing.T) {
	s := newTestKVStore()

	tests := []struct {
		height uint64
		want   bool
	}{
		{0, false},
		{1, false},
		{99, false},
		{100, true},
		{101, false},
		{200, true},
		{300, true},
	}
	for _, tt := range tests {
		got := IsEpochBoundary(s, tt.height)
		if got != tt.want {
			t.Errorf("IsEpochBoundary(%d) = %v, want %v", tt.height, got, tt.want)
		}
	}
}

func TestProcessEpochTransition_ActivateValidators(t *testing.T) {
	s := newTestStakingState()
	addr1, _ := types.AddressFromBytes([]byte("11111111111111111111"))
	addr2, _ := types.AddressFromBytes([]byte("22222222222222222222"))

	// Register validators at epoch 0, they get ActivationEpoch = 1
	fundAccount(s, addr1, 200_000)
	fundAccount(s, addr2, 200_000)
	id1, _ := RegisterValidator(s, [32]byte{1}, addr1, types.NewAmount(100_000), 0, 0)
	id2, _ := RegisterValidator(s, [32]byte{2}, addr2, types.NewAmount(100_000), 0, 0)
	acc1, _ := s.GetAccount(addr1)
	acc1.SubBalance(types.NewAmount(100_000))
	s.SetAccount(addr1, acc1)
	acc2, _ := s.GetAccount(addr2)
	acc2.SubBalance(types.NewAmount(100_000))
	s.SetAccount(addr2, acc2)

	// Both should be pending
	v1, _ := GetValidator(s, id1)
	if v1.Status != ValidatorPending {
		t.Fatalf("v1 status = %s, want pending", v1.Status)
	}

	// Process epoch transition at height 100
	if err := ProcessEpochTransition(s, 100); err != nil {
		t.Fatalf("ProcessEpochTransition failed: %v", err)
	}

	// Both should now be active
	v1, _ = GetValidator(s, id1)
	if v1.Status != ValidatorActive {
		t.Errorf("v1 status = %s, want active after transition", v1.Status)
	}
	v2, _ := GetValidator(s, id2)
	if v2.Status != ValidatorActive {
		t.Errorf("v2 status = %s, want active after transition", v2.Status)
	}

	epoch, _ := CurrentEpoch(s)
	if epoch != 1 {
		t.Errorf("current epoch = %d, want 1", epoch)
	}
}

func TestProcessEpochTransition_RemoveUnstaking(t *testing.T) {
	s := newTestStakingState()
	addr, _ := types.AddressFromBytes([]byte("11111111111111111111"))
	fundAccount(s, addr, 200_000)

	id, _ := RegisterValidator(s, [32]byte{1}, addr, types.NewAmount(100_000), 0, 0)
	acc, _ := s.GetAccount(addr)
	acc.SubBalance(types.NewAmount(100_000))
	s.SetAccount(addr, acc)
	ActivateValidator(s, id, 1)

	// Start unstaking with cooldown
	StartUnstake(s, id, 1) // UnstakeEpoch = 1 (will be removed at epoch 1 transition)

	// Process epoch transition at height 100 (epoch 1)
	if err := ProcessEpochTransition(s, 100); err != nil {
		t.Fatalf("ProcessEpochTransition failed: %v", err)
	}

	// Validator should be removed
	v, err := GetValidator(s, id)
	if err != nil {
		t.Fatalf("GetValidator failed: %v", err)
	}
	if v.Status != ValidatorRemoved {
		t.Errorf("status = %s, want removed", v.Status)
	}

	// Stake should have been returned to account
	acc, _ = s.GetAccount(addr)
	if acc.Balance.Cmp(types.NewAmount(200_000)) != 0 {
		t.Errorf("balance = %s, want 200000 (fully restored)", acc.Balance)
	}
}

func TestProcessEpochTransition_DeferredUnstaking(t *testing.T) {
	s := newTestStakingState()
	addr, _ := types.AddressFromBytes([]byte("11111111111111111111"))
	fundAccount(s, addr, 200_000)

	id, _ := RegisterValidator(s, [32]byte{1}, addr, types.NewAmount(100_000), 0, 0)
	acc, _ := s.GetAccount(addr)
	acc.SubBalance(types.NewAmount(100_000))
	s.SetAccount(addr, acc)
	ActivateValidator(s, id, 1)

	// Start unstaking, cooldown ends at epoch 3
	StartUnstake(s, id, 3)

	// Process epoch 1 transition (height 100) — epoch 0 → 1: validator enters Unstaking
	if err := ProcessEpochTransition(s, 100); err != nil {
		t.Fatalf("ProcessEpochTransition(100) failed: %v", err)
	}

	v, _ := GetValidator(s, id)
	if v.Status != ValidatorUnstaking {
		t.Errorf("status after epoch 1 = %s, want unstaking (not yet eligible)", v.Status)
	}

	// Process epoch 2 transition (height 200) — epoch 1 → 2: still not eligible (UnstakeEpoch=3 > 2)
	if err := ProcessEpochTransition(s, 200); err != nil {
		t.Fatalf("ProcessEpochTransition(200) failed: %v", err)
	}

	v, _ = GetValidator(s, id)
	if v.Status != ValidatorUnstaking {
		t.Errorf("status after epoch 2 = %s, want unstaking (UnstakeEpoch=3 > 2)", v.Status)
	}

	// Process epoch 3 transition (height 300) — epoch 2 → 3: eligible now (UnstakeEpoch=3 <= 3)
	if err := ProcessEpochTransition(s, 300); err != nil {
		t.Fatalf("ProcessEpochTransition(300) failed: %v", err)
	}

	v, _ = GetValidator(s, id)
	if v.Status != ValidatorRemoved {
		t.Errorf("status after epoch 3 = %s, want removed", v.Status)
	}
}

func TestProcessEpochTransition_OnlyActiveAtBoundary(t *testing.T) {
	s := newTestStakingState()
	addr, _ := types.AddressFromBytes([]byte("11111111111111111111"))
	fundAccount(s, addr, 100_000)

	// Register, but don't activate
	RegisterValidator(s, [32]byte{1}, addr, types.NewAmount(100_000), 0, 0)

	// Before epoch transition, check no active validators
	active, _ := GetActiveValidators(s)
	if len(active) != 0 {
		t.Fatalf("active before transition = %d, want 0", len(active))
	}

	// Process epoch 1
	ProcessEpochTransition(s, 100)

	// Validator should now be active (ActivationEpoch was 1)
	active, _ = GetActiveValidators(s)
	if len(active) != 1 {
		t.Errorf("active after transition = %d, want 1", len(active))
	}
}

func TestBlocksPerEpochDefault(t *testing.T) {
	s := newTestKVStore()
	if bpe := BlocksPerEpoch(s); bpe != 100 {
		t.Errorf("default blocks per epoch = %d, want 100", bpe)
	}
}

func TestSetBlocksPerEpoch(t *testing.T) {
	s := newTestKVStore()
	SetBlocksPerEpoch(s, 50)
	if bpe := BlocksPerEpoch(s); bpe != 50 {
		t.Errorf("blocks per epoch = %d, want 50", bpe)
	}
}

func TestSetBlocksPerEpoch_Zero(t *testing.T) {
	s := newTestKVStore()
	if err := SetBlocksPerEpoch(s, 0); err == nil {
		t.Fatal("expected error for zero blocks")
	}
}

func TestUnstakeCooldownDefault(t *testing.T) {
	s := newTestKVStore()
	if cooldown := UnstakeCooldown(s); cooldown != 14 {
		t.Errorf("default unstake cooldown = %d, want 14", cooldown)
	}
}

func TestGetEpochInfo(t *testing.T) {
	s := newTestStakingState()
	addr, _ := types.AddressFromBytes([]byte("11111111111111111111"))
	fundAccount(s, addr, 200_000)

	RegisterValidator(s, [32]byte{1}, addr, types.NewAmount(100_000), 0, 0)
	acc, _ := s.GetAccount(addr)
	acc.SubBalance(types.NewAmount(100_000))
	s.SetAccount(addr, acc)

	ProcessEpochTransition(s, 100)

	info, err := GetEpochInfo(s)
	if err != nil {
		t.Fatal(err)
	}
	if info.CurrentEpoch != 1 {
		t.Errorf("epoch = %d, want 1", info.CurrentEpoch)
	}
	if info.TotalValidators != 1 {
		t.Errorf("total validators = %d, want 1", info.TotalValidators)
	}
	if info.ActiveValidators != 1 {
		t.Errorf("active validators = %d, want 1", info.ActiveValidators)
	}
	if info.TotalBonded.Cmp(types.NewAmount(100_000)) != 0 {
		t.Errorf("total bonded = %s, want 100000", info.TotalBonded)
	}
}
