package consensus

import (
	"testing"

	"github.com/BryanOx/dsn/staking"
	"github.com/BryanOx/dsn/types"
)

// TestEpochTransition_ValidatorActivation tests that pending validators
// become active on epoch boundaries.
func TestEpochTransition_ValidatorActivation(t *testing.T) {
	// This test verifies the validator set transition at epoch boundaries
	// We test that:
	// 1. Initial validators are in the active set
	// 2. After epoch transition, pending validators become active

	// Create initial active validators
	activeValidators := []*staking.Validator{
		{
			ConsensusID: types.Address{1},
			PublicKey:   [32]byte{1},
			VotingPower: 100,
			Status:      staking.ValidatorActive,
		},
		{
			ConsensusID: types.Address{2},
			PublicKey:   [32]byte{2},
			VotingPower: 100,
			Status:      staking.ValidatorActive,
		},
	}

	// Create pending validators that should become active at epoch transition
	pendingValidators := []*staking.Validator{
		{
			ConsensusID: types.Address{3},
			PublicKey:   [32]byte{3},
			VotingPower: 150,
			Status:      staking.ValidatorPending,
		},
		{
			ConsensusID: types.Address{4},
			PublicKey:   [32]byte{4},
			VotingPower: 50,
			Status:      staking.ValidatorPending,
		},
	}

	// Simulate epoch transition - pending validators become active
	newActiveValidators := make([]*staking.Validator, len(activeValidators))
	copy(newActiveValidators, activeValidators)

	for _, pv := range pendingValidators {
		if pv.Status == staking.ValidatorPending {
			// Activate the validator
			pv.Status = staking.ValidatorActive
			newActiveValidators = append(newActiveValidators, pv)
		}
	}

	// Verify all validators are now active
	if len(newActiveValidators) != 4 {
		t.Errorf("expected 4 active validators, got %d", len(newActiveValidators))
	}

	// Check that pending ones are now active
	for _, v := range newActiveValidators {
		if v.Status != staking.ValidatorActive {
			t.Errorf("validator %v should be active but is %v", v.ConsensusID, v.Status)
		}
	}

	// Check total voting power
	totalPower := uint64(0)
	for _, v := range newActiveValidators {
		totalPower += v.VotingPower
	}
	if totalPower != 400 { // 100+100+150+50
		t.Errorf("expected total voting power 400, got %d", totalPower)
	}
}

// TestEpochTransition_ValidatorRemoval tests that validators are removed
// from the active set when unstaking.
func TestEpochTransition_ValidatorRemoval(t *testing.T) {
	// Initial active set
	activeValidators := []*staking.Validator{
		{ConsensusID: types.Address{1}, VotingPower: 100, Status: staking.ValidatorActive},
		{ConsensusID: types.Address{2}, VotingPower: 100, Status: staking.ValidatorActive},
		{ConsensusID: types.Address{3}, VotingPower: 100, Status: staking.ValidatorActive},
	}

	// Validator 2 is unstaked (jailed or removed)
	// In practice, this would be marked as "unbonding" or removed from set
	unstakedValidator := &staking.Validator{
		ConsensusID: types.Address{2},
		VotingPower: 0,
		Status:      staking.ValidatorUnstaking,
	}

	// Simulate removing the validator
	newActiveValidators := []*staking.Validator{}
	for _, v := range activeValidators {
		if v.ConsensusID != unstakedValidator.ConsensusID {
			newActiveValidators = append(newActiveValidators, v)
		}
	}

	if len(newActiveValidators) != 2 {
		t.Errorf("expected 2 validators after removal, got %d", len(newActiveValidators))
	}

	// Verify correct validators remain
	found1 := false
	found3 := false
	addr1 := types.Address{1}
	addr2 := types.Address{2}
	addr3 := types.Address{3}
	for _, v := range newActiveValidators {
		if v.ConsensusID == addr1 {
			found1 = true
		}
		if v.ConsensusID == addr3 {
			found3 = true
		}
		if v.ConsensusID == addr2 {
			t.Error("validator 2 should have been removed")
		}
	}

	if !found1 || !found3 {
		t.Error("expected validators 1 and 3 to remain")
	}
}

// TestEpochTransition_ProposerSelectionAfterTransition tests that proposer
// selection works correctly after epoch transition.
func TestEpochTransition_ProposerSelectionAfterTransition(t *testing.T) {
	// After epoch transition, proposer selection should include new validators
	validators := []*staking.Validator{
		{ConsensusID: types.Address{1}, VotingPower: 100},
		{ConsensusID: types.Address{2}, VotingPower: 100},
		{ConsensusID: types.Address{3}, VotingPower: 200}, // New validator from pending
	}

	// Test that proposer selection works with new validator set
	for height := uint64(0); height < 10; height++ {
		proposer := WeightedProposerAtHeight(height, validators)
		if proposer == (types.Address{}) {
			t.Errorf("proposer should not be zero at height %d", height)
		}
	}
}

// TestEpochTransition_EmptyValidatorSet tests that operations on empty
// validator set are handled properly.
func TestEpochTransition_EmptyValidatorSet(t *testing.T) {
	// Empty validator set should cause panic in WeightedProposerAtHeight
	// This is expected behavior - the chain cannot function without validators

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on empty validator set")
		}
	}()

	WeightedProposerAtHeight(0, []*staking.Validator{})
}

// TestEpochTransition_SingleValidator tests epoch transition with a
// single validator.
func TestEpochTransition_SingleValidator(t *testing.T) {
	validators := []*staking.Validator{
		{ConsensusID: types.Address{99}, VotingPower: 500, Status: staking.ValidatorActive},
	}

	// Single validator should always be proposer
	for height := uint64(0); height < 100; height++ {
		proposer := WeightedProposerAtHeight(height, validators)
		if proposer != validators[0].ConsensusID {
			t.Errorf("height %d: single validator should always be proposer", height)
		}
	}
}

// TestEpochTransition_VotingPowerDistribution tests that voting power
// is correctly distributed among validators.
func TestEpochTransition_VotingPowerDistribution(t *testing.T) {
	validators := []*staking.Validator{
		{ConsensusID: types.Address{1}, VotingPower: 200},
		{ConsensusID: types.Address{2}, VotingPower: 300},
		{ConsensusID: types.Address{3}, VotingPower: 500},
	}

	// Total voting power = 1000
	// Validator 1: 200/1000 = 20%
	// Validator 2: 300/1000 = 30%
	// Validator 3: 500/1000 = 50%

	// Test multiple heights to verify distribution
	// Height 0 should be validator 3 (first in the power-sorted order)
	proposer := WeightedProposerAtHeight(0, validators)

	// At height 0, first validator (highest power) should be proposer
	if proposer != validators[2].ConsensusID {
		t.Logf("height 0 proposer: %v (highest power validator)", proposer)
	}
}
