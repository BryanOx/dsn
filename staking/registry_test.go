package staking

import (
	"testing"

	"github.com/BryanOx/dsn/types"
)

// testKVStore is a simple in-memory KVStore implementation for testing.
type testKVStore struct {
	data map[string][]byte
}

func newTestKVStore() *testKVStore {
	return &testKVStore{data: make(map[string][]byte)}
}

func (s *testKVStore) GetBytes(key string) ([]byte, bool) {
	val, ok := s.data[key]
	return val, ok
}

func (s *testKVStore) SetBytes(key string, value []byte) error {
	s.data[key] = value
	return nil
}

func (s *testKVStore) DeleteBytes(key string) error {
	delete(s.data, key)
	return nil
}

func TestRegisterValidator(t *testing.T) {
	s := newTestKVStore()
	pubKey := [32]byte{1, 2, 3}
	addr, _ := types.AddressFromBytes([]byte("12345678901234567890"))
	stake := types.NewAmount(MinimumValidatorStake)

	id, err := RegisterValidator(s, pubKey, addr, stake, 1000, 0)
	if err != nil {
		t.Fatalf("RegisterValidator failed: %v", err)
	}
	// id is now types.Address (ConsensusID), check that it's not zero
	if id == (types.Address{}) {
		t.Errorf("id = zero address, want non-zero")
	}

	// Check count
	count, err := ValidatorCount(s)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}

	// Check total bonded
	total, err := TotalBonded(s)
	if err != nil {
		t.Fatal(err)
	}
	if total.Cmp(stake) != 0 {
		t.Errorf("total bonded = %s, want %s", total, stake)
	}
}

func TestRegisterValidator_DuplicateAddress(t *testing.T) {
	s := newTestKVStore()
	pubKey := [32]byte{1}
	addr, _ := types.AddressFromBytes([]byte("12345678901234567890"))
	stake := types.NewAmount(MinimumValidatorStake)

	_, err := RegisterValidator(s, pubKey, addr, stake, 0, 0)
	if err != nil {
		t.Fatalf("first register failed: %v", err)
	}

	_, err = RegisterValidator(s, [32]byte{2}, addr, stake, 0, 0)
	if err == nil {
		t.Fatal("expected error for duplicate address")
	}
}

func TestRegisterValidator_BelowMinimumStake(t *testing.T) {
	s := newTestKVStore()
	pubKey := [32]byte{1}
	addr, _ := types.AddressFromBytes([]byte("12345678901234567890"))
	stake := types.NewAmount(MinimumValidatorStake - 1)

	_, err := RegisterValidator(s, pubKey, addr, stake, 0, 0)
	if err == nil {
		t.Fatal("expected error for below minimum stake")
	}
}

func TestGetValidatorByOperator(t *testing.T) {
	s := newTestKVStore()
	pubKey := [32]byte{1, 2, 3}
	addr, _ := types.AddressFromBytes([]byte("12345678901234567890"))
	stake := types.NewAmount(MinimumValidatorStake)

	id, err := RegisterValidator(s, pubKey, addr, stake, 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	v, err := GetValidatorByOperator(s, addr)
	if err != nil {
		t.Fatalf("GetValidatorByOperator failed: %v", err)
	}
	if v.ConsensusID != id {
		t.Errorf("ConsensusID = %v, want %v", v.ConsensusID, id)
	}
}

func TestGetNonExistentValidator(t *testing.T) {
	s := newTestKVStore()
	addr, _ := types.AddressFromBytes([]byte("12345678901234567890"))

	nonExistentID := types.Address{9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9}
	_, err := GetValidator(s, nonExistentID)
	if err == nil {
		t.Fatal("expected error for non-existent validator")
	}

	_, err = GetValidatorByOperator(s, addr)
	if err == nil {
		t.Fatal("expected error for non-existent address")
	}
}

func TestActivateValidator(t *testing.T) {
	s := newTestKVStore()
	pubKey := [32]byte{1}
	addr, _ := types.AddressFromBytes([]byte("12345678901234567890"))
	stake := types.NewAmount(MinimumValidatorStake)

	id, err := RegisterValidator(s, pubKey, addr, stake, 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Activate
	if err := ActivateValidator(s, id, 1); err != nil {
		t.Fatalf("ActivateValidator failed: %v", err)
	}

	v, _ := GetValidator(s, id)
	if v.Status != ValidatorActive {
		t.Errorf("status = %s, want active", v.Status)
	}
	if v.VotingPower == 0 {
		t.Error("voting power should be > 0 for active validator")
	}

	// Should not be able to activate again
	if err := ActivateValidator(s, id, 2); err == nil {
		t.Fatal("should not activate already-active validator")
	}
}

func TestJailAndUnjailValidator(t *testing.T) {
	s := newTestKVStore()
	pubKey := [32]byte{1}
	addr, _ := types.AddressFromBytes([]byte("12345678901234567890"))
	stake := types.NewAmount(MinimumValidatorStake)

	id, _ := RegisterValidator(s, pubKey, addr, stake, 0, 0)
	ActivateValidator(s, id, 1)

	// Jail
	if err := JailValidator(s, id, 5); err != nil {
		t.Fatalf("JailValidator failed: %v", err)
	}
	v, _ := GetValidator(s, id)
	if v.Status != ValidatorJailed {
		t.Errorf("status = %s, want jailed", v.Status)
	}
	if v.VotingPower != 0 {
		t.Error("jailed validator should have 0 voting power")
	}

	// Unjail
	if err := UnjailValidator(s, id); err != nil {
		t.Fatalf("UnjailValidator failed: %v", err)
	}
	v, _ = GetValidator(s, id)
	if v.Status != ValidatorActive {
		t.Errorf("status = %s, want active", v.Status)
	}
	if v.VotingPower == 0 {
		t.Error("unjailed validator should have voting power")
	}
}

func TestUnstakeAndRemoveValidator(t *testing.T) {
	s := newTestKVStore()
	pubKey := [32]byte{1}
	addr, _ := types.AddressFromBytes([]byte("12345678901234567890"))
	stake := types.NewAmount(MinimumValidatorStake)

	id, _ := RegisterValidator(s, pubKey, addr, stake, 0, 0)
	ActivateValidator(s, id, 1)

	// Start unstaking
	if err := StartUnstake(s, id, 14); err != nil {
		t.Fatalf("StartUnstake failed: %v", err)
	}
	v, _ := GetValidator(s, id)
	if v.Status != ValidatorUnstaking {
		t.Errorf("status = %s, want unstaking", v.Status)
	}
	if v.VotingPower != 0 {
		t.Error("unstaking validator should have 0 voting power")
	}
	if v.UnstakeEpoch != 14 {
		t.Errorf("UnstakeEpoch = %d, want 14", v.UnstakeEpoch)
	}

	// Remove
	if err := RemoveValidator(s, id); err != nil {
		t.Fatalf("RemoveValidator failed: %v", err)
	}
	v, _ = GetValidator(s, id)
	if v.Status != ValidatorRemoved {
		t.Errorf("status = %s, want removed", v.Status)
	}

	// Check total bonded decreased
	total, _ := TotalBonded(s)
	if !total.IsZero() {
		t.Errorf("total bonded = %s, want 0 after removal", total)
	}
}

func TestGetActiveValidators_Ordering(t *testing.T) {
	s := newTestKVStore()

	// Register 3 validators with different stakes
	addr1, _ := types.AddressFromBytes([]byte("11111111111111111111"))
	addr2, _ := types.AddressFromBytes([]byte("22222222222222222222"))
	addr3, _ := types.AddressFromBytes([]byte("33333333333333333333"))

	cid1, _ := RegisterValidator(s, [32]byte{1}, addr1, types.NewAmount(100_000), 0, 0)
	cid2, _ := RegisterValidator(s, [32]byte{2}, addr2, types.NewAmount(200_000), 0, 0)
	cid3, _ := RegisterValidator(s, [32]byte{3}, addr3, types.NewAmount(150_000), 0, 0)

	// Activate all
	ActivateValidator(s, cid1, 1)
	ActivateValidator(s, cid2, 1)
	ActivateValidator(s, cid3, 1)

	active, err := GetActiveValidators(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 3 {
		t.Fatalf("got %d active validators, want 3", len(active))
	}

	// Expected order: id2 (200k), id3 (150k), id1 (100k) by voting power DESC
	if active[0].ConsensusID != cid2 {
		t.Errorf("first = ConsensusID %v, want cid2 (%v)", active[0].ConsensusID, cid2)
	}
	if active[1].ConsensusID != cid3 {
		t.Errorf("second = ConsensusID %v, want cid3 (%v)", active[1].ConsensusID, cid3)
	}
	if active[2].ConsensusID != cid1 {
		t.Errorf("third = ConsensusID %v, want cid1 (%v)", active[2].ConsensusID, cid1)
	}
}

func TestGetActiveValidators_OnlyActive(t *testing.T) {
	s := newTestKVStore()
	addr, _ := types.AddressFromBytes([]byte("12345678901234567890"))

	id, _ := RegisterValidator(s, [32]byte{1}, addr, types.NewAmount(100_000), 0, 0)
	// NOT activated — should not appear in active set

	active, err := GetActiveValidators(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 0 {
		t.Errorf("got %d active validators, want 0 before activation", len(active))
	}

	ActivateValidator(s, id, 1)
	active, _ = GetActiveValidators(s)
	if len(active) != 1 {
		t.Errorf("got %d active validators, want 1 after activation", len(active))
	}
}

func TestTotalBonded_Accumulates(t *testing.T) {
	s := newTestKVStore()
	addr1, _ := types.AddressFromBytes([]byte("11111111111111111111"))
	addr2, _ := types.AddressFromBytes([]byte("22222222222222222222"))

	RegisterValidator(s, [32]byte{1}, addr1, types.NewAmount(100_000), 0, 0)
	total, _ := TotalBonded(s)
	if total.Cmp(types.NewAmount(100_000)) != 0 {
		t.Errorf("total = %s, want 100000", total)
	}

	RegisterValidator(s, [32]byte{2}, addr2, types.NewAmount(200_000), 0, 0)
	total, _ = TotalBonded(s)
	if total.Cmp(types.NewAmount(300_000)) != 0 {
		t.Errorf("total = %s, want 300000", total)
	}
}

func TestInvalidTransitions_Registry(t *testing.T) {
	s := newTestKVStore()
	addr, _ := types.AddressFromBytes([]byte("12345678901234567890"))
	id, _ := RegisterValidator(s, [32]byte{1}, addr, types.NewAmount(100_000), 0, 0)

	// Can't jail a pending validator
	if err := JailValidator(s, id, 5); err == nil {
		t.Fatal("should not jail pending validator")
	}

	// Can't unjail a non-jailed validator
	if err := UnjailValidator(s, id); err == nil {
		t.Fatal("should not unjail non-jailed validator")
	}

	// Can't remove a non-unstaking validator
	if err := RemoveValidator(s, id); err == nil {
		t.Fatal("should not remove non-unstaking validator")
	}
}
